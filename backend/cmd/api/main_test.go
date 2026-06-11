package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zzyhdu/soniq/backend/internal/api"
	"github.com/zzyhdu/soniq/backend/internal/config"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/client"
)

func TestBuildHandlerCreatesRecordingSessionWithoutStartingWorkflow(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	store := newBuildHandlerRecordingStoreSpy()
	storeFactory := &appStoreFactorySpy{store: store}
	cfg := config.Config{
		APIAddress:        ":0",
		PostgresDSN:       "postgres://custom_user:***@db:5432/custom?sslmode=disable",
		TemporalAddress:   "temporal.example:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "soniq-audio-pipeline",
		StorageProvider:   "local",
		LocalStoragePath:  t.TempDir(),
	}

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(ctx context.Context, cfg config.Config) (temporalWorkflowClient, error) {
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

func TestBuildHandlerWiresUploadEndpointToLocalObjectStorage(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	store := newBuildHandlerRecordingStoreSpy()
	storeFactory := &appStoreFactorySpy{store: store}
	uploadRoot := t.TempDir()
	cfg := config.Config{
		PostgresDSN:       "postgres://custom_user:***@db:5432/custom?sslmode=disable",
		TemporalAddress:   "temporal.example:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "soniq-audio-pipeline",
		StorageProvider:   "local",
		LocalStoragePath:  uploadRoot,
	}

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(context.Context, config.Config) (temporalWorkflowClient, error) {
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
		t.Fatal("AudioObjectKey is empty, want stored local object key")
	}
	if recording.AudioContentType != "audio/wav" {
		t.Fatalf("AudioContentType = %q, want audio/wav", recording.AudioContentType)
	}
	if recording.AudioSizeBytes != int64(len("audio-bytes")) {
		t.Fatalf("AudioSizeBytes = %d, want %d", recording.AudioSizeBytes, len("audio-bytes"))
	}
	storedBytes, err := os.ReadFile(filepath.Join(uploadRoot, filepath.FromSlash(recording.AudioObjectKey)))
	if err != nil {
		t.Fatalf("read uploaded object: %v", err)
	}
	if string(storedBytes) != "audio-bytes" {
		t.Fatalf("stored object = %q, want audio-bytes", string(storedBytes))
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

	_, cleanup, err := buildHandler(context.Background(), config.Config{TemporalTaskQueue: "soniq-audio-pipeline", PostgresDSN: "postgres://custom_user:***@db:5432/custom?sslmode=disable", StorageProvider: "local", LocalStoragePath: t.TempDir()}, func(context.Context, config.Config) (temporalWorkflowClient, error) {
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
	stored map[string]domain.Recording
	nextID int
	closed bool
}

func newBuildHandlerRecordingStoreSpy() *buildHandlerRecordingStoreSpy {
	return &buildHandlerRecordingStoreSpy{stored: make(map[string]domain.Recording)}
}

func (s *buildHandlerRecordingStoreSpy) RecordingStore() api.RecordingDetailsStore {
	return s
}

func (s *buildHandlerRecordingStoreSpy) WorkspaceStore() api.WorkspaceStore {
	return s
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
	if !ok || recording.WorkspaceID != input.WorkspaceID {
		return domain.Recording{}, false, nil
	}
	return recording, true, nil
}

func (s *buildHandlerRecordingStoreSpy) ListByWorkspace(input recordings.ListRecordingsInput) ([]domain.Recording, error) {
	result := []domain.Recording{}
	for _, recording := range s.stored {
		if recording.WorkspaceID == input.WorkspaceID {
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

type temporalClientSpy struct {
	calls  []workflowStartCall
	closed bool
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

func (s *temporalClientSpy) Close() {
	s.closed = true
}
