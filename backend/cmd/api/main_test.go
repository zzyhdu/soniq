package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/api"
	"github.com/zzyhdu/soniq/backend/internal/auth"
	"github.com/zzyhdu/soniq/backend/internal/config"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/observability"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/client"
)

func TestServeHTTPServerShutdownWaitsForInFlightRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(handlerStarted)
		<-releaseHandler
		w.WriteHeader(http.StatusAccepted)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- serveHTTPServer(ctx, nil, server, listener, time.Second)
	}()

	responseErr := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			responseErr <- err
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			responseErr <- fmt.Errorf("status code = %d, want %d", response.StatusCode, http.StatusAccepted)
			return
		}
		responseErr <- nil
	}()

	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler to start")
	}

	cancel()
	select {
	case err := <-serverErr:
		t.Fatalf("server returned before in-flight handler finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseHandler)
	select {
	case err := <-responseErr:
		if err != nil {
			t.Fatalf("request error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for request to complete")
	}
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for graceful shutdown")
	}
}

func TestBuildHandlerCreatesRecordingSessionWithoutStartingWorkflow(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	store := newBuildHandlerRecordingStoreSpy()
	enableBuildHandlerPassword(t, store)
	storeFactory := &appStoreFactorySpy{store: store}
	cfg := withTestS3Config(config.Config{
		APIAddress:        ":0",
		PostgresDSN:       "postgres://custom_user:***@db:5432/custom?sslmode=disable",
		TemporalAddress:   "temporal.example:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "soniq-audio-pipeline",
	}, "http://127.0.0.1:1")

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(ctx context.Context, cfg config.Config, tracing *observability.Tracing) (temporalWorkflowClient, error) {
		if cfg.TemporalAddress != "temporal.example:7233" {
			t.Fatalf("TemporalAddress = %q, want temporal.example:7233", cfg.TemporalAddress)
		}
		if cfg.TemporalNamespace != "default" {
			t.Fatalf("TemporalNamespace = %q, want default", cfg.TemporalNamespace)
		}
		return temporalClient, nil
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()
	if got, want := len(storeFactory.calls), 1; got != want {
		t.Fatalf("recording store factory calls = %d, want %d", got, want)
	}
	if storeFactory.calls[0] != cfg.PostgresDSN {
		t.Fatalf("recording store factory DSN = %q, want %q", storeFactory.calls[0], cfg.PostgresDSN)
	}

	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings", strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`))
	request.Header.Set("Content-Type", "application/json")
	addHandlerAuth(t, request, buildHandlerAuthCookies(t, handler))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var recording domain.Recording
	if err := json.NewDecoder(response.Body).Decode(&recording); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got, want := len(temporalClient.calls), 0; got != want {
		t.Fatalf("ExecuteWorkflow calls = %d, want %d", got, want)
	}
}

func TestBuildHandlerWiresUploadEndpointToS3CompatibleObjectStorage(t *testing.T) {
	var storedObjectKey string
	var storedContentType string
	var storedBody string
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("s3 request method = %s, want PUT", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/soniq/") {
			t.Fatalf("s3 request path = %q, want /soniq/<object-key>", r.URL.Path)
		}
		storedObjectKey = strings.TrimPrefix(r.URL.Path, "/soniq/")
		storedContentType = r.Header.Get("Content-Type")
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read s3 request body: %v", err)
		}
		storedBody = string(rawBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	temporalClient := &temporalClientSpy{}
	store := newBuildHandlerRecordingStoreSpy()
	enableBuildHandlerPassword(t, store)
	storeFactory := &appStoreFactorySpy{store: store}
	cfg := withTestS3Config(config.Config{
		PostgresDSN:       "postgres://custom_user:***@db:5432/custom?sslmode=disable",
		TemporalAddress:   "temporal.example:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "soniq-audio-pipeline",
	}, s3Server.URL)

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(context.Context, config.Config, *observability.Tracing) (temporalWorkflowClient, error) {
		return temporalClient, nil
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	request := newMultipartUploadRequest(t, "/workspaces/wsp_default/recordings/upload", map[string]string{
		"title":         "Weekly sync",
		"workflow_type": "meeting",
		"language":      "en",
	}, "audio", "weekly.wav", "audio/wav", "audio-bytes")
	addHandlerAuth(t, request, buildHandlerAuthCookies(t, handler))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var body struct {
		Recording          domain.Recording `json:"recording"`
		ProcessingEnqueued bool             `json:"processing_enqueued"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if !body.ProcessingEnqueued {
		t.Fatal("processing_enqueued = false, want true")
	}
	recording := body.Recording
	if recording.AudioObjectKey == "" {
		t.Fatal("AudioObjectKey is empty, want stored object key")
	}
	if recording.AudioContentType != "audio/wav" {
		t.Fatalf("AudioContentType = %q, want audio/wav", recording.AudioContentType)
	}
	if recording.AudioSizeBytes != int64(len("audio-bytes")) {
		t.Fatalf("AudioSizeBytes = %d, want %d", recording.AudioSizeBytes, len("audio-bytes"))
	}
	if storedObjectKey != recording.AudioObjectKey {
		t.Fatalf("stored object key = %q, want %q", storedObjectKey, recording.AudioObjectKey)
	}
	if storedContentType != "audio/wav" {
		t.Fatalf("stored content type = %q, want audio/wav", storedContentType)
	}
	if storedBody != "audio-bytes" {
		t.Fatalf("stored object = %q, want audio-bytes", storedBody)
	}
	if got, want := len(temporalClient.calls), 1; got != want {
		t.Fatalf("ExecuteWorkflow calls = %d, want %d", got, want)
	}
	call := temporalClient.calls[0]
	if call.options.ID != "recording-processing-"+recording.ID {
		t.Fatalf("workflow ID = %q, want recording-processing-%s", call.options.ID, recording.ID)
	}
	if call.options.TaskQueue != "soniq-audio-pipeline" {
		t.Fatalf("task queue = %q, want soniq-audio-pipeline", call.options.TaskQueue)
	}
	if !sameFunction(call.workflow, workflows.RecordingProcessingWorkflow) {
		t.Fatalf("workflow = %T, want RecordingProcessingWorkflow", call.workflow)
	}
	input, ok := call.args[0].(workflows.RecordingProcessingInput)
	if !ok {
		t.Fatalf("workflow arg type = %T, want RecordingProcessingInput", call.args[0])
	}
	if input.RecordingID != recording.ID {
		t.Fatalf("input recording ID = %q, want %q", input.RecordingID, recording.ID)
	}
	if input.WorkspaceID != recording.WorkspaceID {
		t.Fatalf("input workspace ID = %q, want %q", input.WorkspaceID, recording.WorkspaceID)
	}
	if input.WorkflowType != domain.WorkflowTypeMeeting {
		t.Fatalf("input workflow_type = %q, want meeting", input.WorkflowType)
	}
	if input.Language != "en" {
		t.Fatalf("input language = %q, want en", input.Language)
	}
}

func TestBuildHandlerCleanupClosesTemporalClient(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	store := newBuildHandlerRecordingStoreSpy()
	storeFactory := &appStoreFactorySpy{store: store}

	_, cleanup, err := buildHandler(context.Background(), withTestS3Config(config.Config{TemporalTaskQueue: "soniq-audio-pipeline", PostgresDSN: "postgres://custom_user:***@db:5432/custom?sslmode=disable"}, "http://127.0.0.1:1"), func(context.Context, config.Config, *observability.Tracing) (temporalWorkflowClient, error) {
		return temporalClient, nil
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}

	cleanup()

	if !temporalClient.closed {
		t.Fatal("temporal client closed = false, want true")
	}
	if !store.closed {
		t.Fatal("recording store closed = false, want true")
	}
}

func TestBuildHandlerLoginSessionCanReadMe(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	store := newBuildHandlerRecordingStoreSpy()
	enableBuildHandlerPassword(t, store)
	storeFactory := &appStoreFactorySpy{store: store}
	cfg := withTestS3Config(config.Config{
		AuthSessionTTLHours: 24,
		PostgresDSN:         "postgres://custom_user:***@db:5432/custom?sslmode=disable",
		TemporalAddress:     "temporal.example:7233",
		TemporalNamespace:   "default",
		TemporalTaskQueue:   "soniq-audio-pipeline",
	}, "http://127.0.0.1:1")

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(context.Context, config.Config, *observability.Tracing) (temporalWorkflowClient, error) {
		return temporalClient, nil
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	meRequest := httptest.NewRequest(http.MethodGet, "/me", nil)
	meRequest.AddCookie(buildHandlerAuthCookies(t, handler).session)
	meResponse := httptest.NewRecorder()
	handler.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("me status code = %d, want %d; body=%s", meResponse.Code, http.StatusOK, meResponse.Body.String())
	}
}

func TestBuildHandlerReadyzChecksRuntimeDependencies(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	s3Server := newHeadBucketServer(t)
	defer s3Server.Close()
	store := newBuildHandlerRecordingStoreSpy()
	enableBuildHandlerPassword(t, store)
	storeFactory := &appStoreFactorySpy{store: store}
	cfg := withTestS3Config(config.Config{
		PostgresDSN:       "postgres://custom_user:***@db:5432/custom?sslmode=disable",
		TemporalAddress:   "temporal.example:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "soniq-audio-pipeline",
	}, s3Server.URL)

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(context.Context, config.Config, *observability.Tracing) (temporalWorkflowClient, error) {
		return temporalClient, nil
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("readyz status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode readyz body: %v", err)
	}
	if body.Status != "ready" {
		t.Fatalf("readyz status = %q, want ready", body.Status)
	}
	for _, checkName := range []string{"postgres", "migrations", "temporal", "object_storage"} {
		if body.Checks[checkName] != "ok" {
			t.Fatalf("readyz check %s = %q, want ok; body=%s", checkName, body.Checks[checkName], response.Body.String())
		}
	}
}

func TestBuildHandlerReadyzChecksS3CompatibleStorage(t *testing.T) {
	var sawHeadBucket bool
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Path != "/soniq" {
			t.Fatalf("s3 readiness request = %s %s, want HEAD /soniq", r.Method, r.URL.Path)
		}
		sawHeadBucket = true
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	temporalClient := &temporalClientSpy{}
	store := newBuildHandlerRecordingStoreSpy()
	enableBuildHandlerPassword(t, store)
	storeFactory := &appStoreFactorySpy{store: store}
	cfg := config.Config{
		PostgresDSN:       "postgres://custom_user:***@db:5432/custom?sslmode=disable",
		TemporalAddress:   "temporal.example:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "soniq-audio-pipeline",
		StorageProvider:   "s3_compatible",
		S3Endpoint:        s3Server.URL,
		S3Region:          "us-east-1",
		S3Bucket:          "soniq",
		S3AccessKey:       "test-access",
		S3SecretKey:       "test-secret",
		S3ForcePathStyle:  true,
	}

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(context.Context, config.Config, *observability.Tracing) (temporalWorkflowClient, error) {
		return temporalClient, nil
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("readyz status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if !sawHeadBucket {
		t.Fatal("s3 HEAD bucket was not called")
	}
}

func TestBuildHandlerReadyzReportsOutdatedMigrations(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	s3Server := newHeadBucketServer(t)
	defer s3Server.Close()
	store := newBuildHandlerRecordingStoreSpy()
	store.migrationVersion = requiredSchemaMigrationVersion - 1
	enableBuildHandlerPassword(t, store)
	storeFactory := &appStoreFactorySpy{store: store}
	cfg := withTestS3Config(config.Config{
		PostgresDSN:       "postgres://custom_user:***@db:5432/custom?sslmode=disable",
		TemporalAddress:   "temporal.example:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "soniq-audio-pipeline",
	}, s3Server.URL)

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(context.Context, config.Config, *observability.Tracing) (temporalWorkflowClient, error) {
		return temporalClient, nil
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status code = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
		Errors map[string]string `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode readyz body: %v", err)
	}
	if body.Status != "not_ready" {
		t.Fatalf("readyz status = %q, want not_ready", body.Status)
	}
	if body.Checks["migrations"] != "error" || body.Errors["migrations"] == "" {
		t.Fatalf("readyz migrations check = %q error=%q, want error with message", body.Checks["migrations"], body.Errors["migrations"])
	}
}

func enableBuildHandlerPassword(t *testing.T, store *buildHandlerRecordingStoreSpy) {
	t.Helper()

	passwordHash, err := auth.HashPassword("correct horse")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	store.auth.passwordHash = passwordHash
}

func withTestS3Config(cfg config.Config, endpoint string) config.Config {
	cfg.StorageProvider = "s3_compatible"
	cfg.S3Endpoint = endpoint
	cfg.S3Region = "us-east-1"
	cfg.S3Bucket = "soniq"
	cfg.S3AccessKey = "test-access"
	cfg.S3SecretKey = "test-secret"
	cfg.S3ForcePathStyle = true
	return cfg
}

func newHeadBucketServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Path != "/soniq" {
			t.Fatalf("s3 readiness request = %s %s, want HEAD /soniq", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
}

type handlerAuthCookies struct {
	session *http.Cookie
	csrf    *http.Cookie
}

func buildHandlerAuthCookies(t *testing.T, handler http.Handler) handlerAuthCookies {
	t.Helper()

	loginRequest := httptest.NewRequest(http.MethodPost, "/auth/signin", strings.NewReader(`{"email":"dev@local.soniq","password":"correct horse"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status code = %d, want %d; body=%s", loginResponse.Code, http.StatusOK, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	var authCookies handlerAuthCookies
	for _, cookie := range cookies {
		switch cookie.Name {
		case api.DefaultSessionCookieName:
			authCookies.session = cookie
		case api.DefaultCSRFCookieName:
			authCookies.csrf = cookie
		}
	}
	if authCookies.session == nil || authCookies.csrf == nil {
		t.Fatalf("login cookies = %+v, want session and csrf cookies", cookies)
	}
	return authCookies
}

func addHandlerAuth(t *testing.T, request *http.Request, authCookies handlerAuthCookies) {
	t.Helper()

	request.AddCookie(authCookies.session)
	request.AddCookie(authCookies.csrf)
	request.Header.Set(api.CSRFHeaderName, authCookies.csrf.Value)
}

func sameFunction(a, b interface{}) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

func newMultipartUploadRequest(t *testing.T, target string, fields map[string]string, fileField, fileName, contentType, fileContents string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("WriteField(%q): %v", name, err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="`+fileField+`"; filename="`+fileName+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write([]byte(fileContents)); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

type appStoreFactorySpy struct {
	store *buildHandlerRecordingStoreSpy
	calls []string
}

func (s *appStoreFactorySpy) Open(ctx context.Context, dsn string) (appStoreClient, error) {
	s.calls = append(s.calls, dsn)
	return s.store, nil
}

type buildHandlerRecordingStoreSpy struct {
	stored           map[string]domain.Recording
	auth             *buildHandlerAuthStoreSpy
	nextID           int
	migrationVersion int
	pingErr          error
	migrationErr     error
	closed           bool
}

func newBuildHandlerRecordingStoreSpy() *buildHandlerRecordingStoreSpy {
	return &buildHandlerRecordingStoreSpy{
		stored: make(map[string]domain.Recording),
		auth:   newBuildHandlerAuthStoreSpy(),
	}
}

func (s *buildHandlerRecordingStoreSpy) RecordingStore() api.RecordingStore {
	return s
}

func (s *buildHandlerRecordingStoreSpy) WorkspaceStore() api.WorkspaceStore {
	return s
}

func (s *buildHandlerRecordingStoreSpy) AuthStore() appAuthStore {
	return s.auth
}

func (s *buildHandlerRecordingStoreSpy) Ping(context.Context) error {
	return s.pingErr
}

func (s *buildHandlerRecordingStoreSpy) LatestSchemaMigrationVersion(context.Context) (int, error) {
	if s.migrationErr != nil {
		return 0, s.migrationErr
	}
	if s.migrationVersion == 0 {
		return requiredSchemaMigrationVersion, nil
	}
	return s.migrationVersion, nil
}

func (s *buildHandlerRecordingStoreSpy) Create(input recordings.CreateRecordingInput) (domain.Recording, error) {
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return domain.Recording{}, errors.New("invalid workflow type")
	}
	s.nextID++
	recording := domain.Recording{
		ID:               fmt.Sprintf("rec_build_%d", s.nextID),
		WorkspaceID:      input.WorkspaceID,
		Title:            input.Title,
		Status:           domain.RecordingStatusUploaded,
		WorkflowType:     input.WorkflowType,
		Language:         input.Language,
		AudioObjectKey:   input.AudioObjectKey,
		AudioContentType: input.AudioContentType,
		AudioSizeBytes:   input.AudioSizeBytes,
	}
	s.stored[recording.ID] = recording
	return recording, nil
}

func (s *buildHandlerRecordingStoreSpy) Get(id string) (domain.Recording, bool, error) {
	recording, ok := s.stored[id]
	return recording, ok, nil
}

func (s *buildHandlerRecordingStoreSpy) GetForWorkspace(input recordings.GetRecordingInput) (domain.Recording, bool, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt != nil {
		return domain.Recording{}, false, nil
	}
	return recording, true, nil
}

func (s *buildHandlerRecordingStoreSpy) ListByWorkspace(input recordings.ListRecordingsInput) ([]domain.Recording, error) {
	result := []domain.Recording{}
	for _, recording := range s.stored {
		if recording.WorkspaceID == input.WorkspaceID && recording.DeletedAt == nil {
			result = append(result, recording)
		}
	}
	return result, nil
}

func (s *buildHandlerRecordingStoreSpy) GetTranscript(string) (recordings.RecordingTranscript, bool, error) {
	return recordings.RecordingTranscript{}, false, nil
}

func (s *buildHandlerRecordingStoreSpy) ListTranscriptSegments(string) ([]recordings.RecordingTranscriptSegment, error) {
	return []recordings.RecordingTranscriptSegment{}, nil
}

func (s *buildHandlerRecordingStoreSpy) GetSummary(string) (recordings.RecordingSummary, bool, error) {
	return recordings.RecordingSummary{}, false, nil
}

func (s *buildHandlerRecordingStoreSpy) GetMindMap(string) (recordings.RecordingMindMap, bool, error) {
	return recordings.RecordingMindMap{}, false, nil
}

func (s *buildHandlerRecordingStoreSpy) ResetForRetry(input recordings.RetryRecordingInput) (domain.Recording, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID {
		return domain.Recording{}, errors.New("recording not found")
	}
	recording.Status = domain.RecordingStatusUploaded
	recording.FailureReason = ""
	recording.FailedAt = nil
	recording.CompletedAt = nil
	s.stored[input.ID] = recording
	return recording, nil
}

func (s *buildHandlerRecordingStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID {
		return domain.Recording{}, errors.New("recording not found")
	}
	recording.Status = input.Status
	recording.FailureReason = input.FailureReason
	s.stored[input.ID] = recording
	return recording, nil
}

func (s *buildHandlerRecordingStoreSpy) SoftDeleteForWorkspace(input recordings.SoftDeleteRecordingInput) (domain.Recording, bool, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt != nil {
		return domain.Recording{}, false, nil
	}
	now := time.Now().UTC()
	recording.DeletedAt = &now
	recording.DeletedByUserID = input.DeletedByUserID
	s.stored[input.ID] = recording
	return recording, true, nil
}

func (s *buildHandlerRecordingStoreSpy) ListDeletedByWorkspace(input recordings.ListDeletedRecordingsInput) ([]domain.Recording, error) {
	result := []domain.Recording{}
	for _, recording := range s.stored {
		if recording.WorkspaceID == input.WorkspaceID && recording.DeletedAt != nil {
			result = append(result, recording)
		}
	}
	return result, nil
}

func (s *buildHandlerRecordingStoreSpy) RestoreForWorkspace(input recordings.RestoreRecordingInput) (domain.Recording, bool, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt == nil {
		return domain.Recording{}, false, nil
	}
	recording.DeletedAt = nil
	recording.DeletedByUserID = ""
	s.stored[input.ID] = recording
	return recording, true, nil
}

func (s *buildHandlerRecordingStoreSpy) PurgeForWorkspace(input recordings.PurgeRecordingInput) (recordings.PurgeRecordingResult, bool, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt == nil {
		return recordings.PurgeRecordingResult{}, false, nil
	}
	delete(s.stored, input.ID)
	return recordings.PurgeRecordingResult{}, true, nil
}

func (s *buildHandlerRecordingStoreSpy) MarkPurgeArtifactDeleted(recordings.MarkPurgeArtifactDeletedInput) (bool, error) {
	return true, nil
}

func (s *buildHandlerRecordingStoreSpy) MarkPurgeArtifactFailed(recordings.MarkPurgeArtifactFailedInput) (bool, error) {
	return true, nil
}

func (s *buildHandlerRecordingStoreSpy) GetUser(_ context.Context, userID string) (domain.User, bool, error) {
	if userID != "usr_dev" {
		return domain.User{}, false, nil
	}
	return domain.User{ID: "usr_dev", Email: "dev@local.soniq", DisplayName: "Local Developer"}, true, nil
}

func (s *buildHandlerRecordingStoreSpy) ListWorkspacesForUser(_ context.Context, userID string) ([]domain.WorkspaceWithRole, error) {
	if userID != "usr_dev" {
		return []domain.WorkspaceWithRole{}, nil
	}
	return []domain.WorkspaceWithRole{{
		ID:              "wsp_default",
		Name:            "Default Workspace",
		CreatedByUserID: "usr_dev",
		Role:            domain.WorkspaceRoleOwner,
	}}, nil
}

func (s *buildHandlerRecordingStoreSpy) GetWorkspaceForUser(_ context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error) {
	if userID != "usr_dev" || workspaceID != "wsp_default" {
		return domain.WorkspaceWithRole{}, false, nil
	}
	return domain.WorkspaceWithRole{
		ID:              "wsp_default",
		Name:            "Default Workspace",
		CreatedByUserID: "usr_dev",
		Role:            domain.WorkspaceRoleOwner,
	}, true, nil
}

func (s *buildHandlerRecordingStoreSpy) Close() {
	s.closed = true
}

type buildHandlerAuthStoreSpy struct {
	user         domain.User
	passwordHash string
	sessions     []auth.CreateSessionInput
}

func newBuildHandlerAuthStoreSpy() *buildHandlerAuthStoreSpy {
	return &buildHandlerAuthStoreSpy{
		user: domain.User{
			ID:          "usr_dev",
			Email:       "dev@local.soniq",
			DisplayName: "Local Developer",
		},
	}
}

func (s *buildHandlerAuthStoreSpy) GetUserByEmail(_ context.Context, email string) (domain.User, string, bool, error) {
	if email != s.user.Email {
		return domain.User{}, "", false, nil
	}
	return s.user, s.passwordHash, true, nil
}

func (s *buildHandlerAuthStoreSpy) SignUp(_ context.Context, input auth.SignUpInput) (domain.User, error) {
	s.user.Email = input.Email
	s.user.DisplayName = input.DisplayName
	s.passwordHash = input.PasswordHash
	return s.user, nil
}

func (s *buildHandlerAuthStoreSpy) CreateSession(_ context.Context, input auth.CreateSessionInput) (auth.Session, error) {
	s.sessions = append(s.sessions, input)
	return auth.Session{
		ID:        "ses_test",
		UserID:    input.UserID,
		TokenHash: input.TokenHash,
		ExpiresAt: input.ExpiresAt,
	}, nil
}

func (s *buildHandlerAuthStoreSpy) GetActiveSessionByTokenHash(_ context.Context, tokenHash string, now time.Time) (auth.Session, bool, error) {
	for _, session := range s.sessions {
		if session.TokenHash == tokenHash && session.ExpiresAt.After(now) {
			return auth.Session{
				ID:        "ses_test",
				UserID:    session.UserID,
				TokenHash: session.TokenHash,
				ExpiresAt: session.ExpiresAt,
			}, true, nil
		}
	}
	return auth.Session{}, false, nil
}

func (s *buildHandlerAuthStoreSpy) RevokeSession(context.Context, string, time.Time) error {
	return nil
}

type temporalClientSpy struct {
	calls     []workflowStartCall
	healthErr error
	closed    bool
}

type workflowStartCall struct {
	options  client.StartWorkflowOptions
	workflow interface{}
	args     []interface{}
}

func (s *temporalClientSpy) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	s.calls = append(s.calls, workflowStartCall{
		options:  options,
		workflow: workflow,
		args:     args,
	})
	return nil, nil
}

func (s *temporalClientSpy) CheckHealth(context.Context, *client.CheckHealthRequest) (*client.CheckHealthResponse, error) {
	if s.healthErr != nil {
		return nil, s.healthErr
	}
	return &client.CheckHealthResponse{}, nil
}

func (s *temporalClientSpy) Close() {
	s.closed = true
}
