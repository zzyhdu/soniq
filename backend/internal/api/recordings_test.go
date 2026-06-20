package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

var errRecordingProcessorFailed = errors.New("recording processor failed")
var errObjectStoreFailed = errors.New("object store failed")
var errRecordingGetFailed = errors.New("recording get failed")
var errRecordingDetailsFailed = errors.New("recording details failed")

const (
	testUserID      = "usr_dev"
	testWorkspaceID = "wsp_default"
)

type recordingProcessorSpy struct {
	enqueued    []domain.Recording
	contexts    []context.Context
	contextErrs []error
	err         error
}

type recordingRequestContextKey struct{}

type fakeRecordingStore struct {
	created              []recordings.CreateRecordingInput
	stored               map[string]domain.Recording
	details              map[string]recordingDetailsFixture
	normalizedObjectKeys map[string]string
	purgeArtifacts       map[string]recordings.RecordingPurgeArtifact
	nextID               int
	createErr            error
	updateErr            error
}

type fakeWorkspaceStore struct {
	user        domain.User
	workspaces  []domain.WorkspaceWithRole
	memberships map[string]domain.WorkspaceWithRole
	getUserErr  error
	listErr     error
	getErr      error
}

type recordingDetailsFixture struct {
	transcript    recordings.RecordingTranscript
	segments      []recordings.RecordingTranscriptSegment
	summary       recordings.RecordingSummary
	mindMap       recordings.RecordingMindMap
	hasTranscript bool
	hasSummary    bool
	hasMindMap    bool
	transcriptErr error
	segmentsErr   error
	summaryErr    error
	mindMapErr    error
}

type getErrRecordingStore struct {
	err error
}

type objectStoreSpy struct {
	puts      []storedObject
	deletes   []string
	err       error
	deleteErr error
}

type storedObject struct {
	key         string
	contentType string
	body        string
}

func newFakeRecordingStore() *fakeRecordingStore {
	return &fakeRecordingStore{
		stored:               make(map[string]domain.Recording),
		details:              make(map[string]recordingDetailsFixture),
		normalizedObjectKeys: make(map[string]string),
		purgeArtifacts:       make(map[string]recordings.RecordingPurgeArtifact),
	}
}

func newFakeWorkspaceStore() *fakeWorkspaceStore {
	workspace := domain.WorkspaceWithRole{
		ID:              testWorkspaceID,
		Name:            "Default Workspace",
		CreatedByUserID: testUserID,
		Role:            domain.WorkspaceRoleOwner,
	}
	return &fakeWorkspaceStore{
		user: domain.User{
			ID:          testUserID,
			Email:       "dev@local.soniq",
			DisplayName: "Local Developer",
		},
		workspaces:  []domain.WorkspaceWithRole{workspace},
		memberships: map[string]domain.WorkspaceWithRole{testUserID + ":" + testWorkspaceID: workspace},
	}
}

func (s *fakeRecordingStore) Create(input recordings.CreateRecordingInput) (domain.Recording, error) {
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return domain.Recording{}, errors.New("invalid workflow type")
	}
	if s.createErr != nil {
		return domain.Recording{}, s.createErr
	}
	s.created = append(s.created, input)
	s.nextID++
	recording := domain.Recording{
		ID:               fmt.Sprintf("rec_fake_%d", s.nextID),
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

func (s *fakeRecordingStore) Get(id string) (domain.Recording, bool, error) {
	recording, ok := s.stored[id]
	if ok && recording.DeletedAt != nil {
		return domain.Recording{}, false, nil
	}
	return recording, ok, nil
}

func (s *fakeRecordingStore) GetForWorkspace(input recordings.GetRecordingInput) (domain.Recording, bool, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt != nil {
		return domain.Recording{}, false, nil
	}
	return recording, true, nil
}

func (s *fakeRecordingStore) ListByWorkspace(input recordings.ListRecordingsInput) ([]domain.Recording, error) {
	result := []domain.Recording{}
	for _, recording := range s.stored {
		if recording.WorkspaceID == input.WorkspaceID && recording.DeletedAt == nil {
			result = append(result, recording)
		}
	}
	return result, nil
}

func (s *fakeRecordingStore) UpdateForWorkspace(input recordings.UpdateRecordingInput) (domain.Recording, bool, error) {
	if s.updateErr != nil {
		return domain.Recording{}, false, s.updateErr
	}
	if strings.TrimSpace(input.Title) == "" {
		return domain.Recording{}, false, errors.New("title is required")
	}
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt != nil {
		return domain.Recording{}, false, nil
	}
	recording.Title = strings.TrimSpace(input.Title)
	recording.UpdatedAt = time.Now().UTC()
	s.stored[input.ID] = recording
	return recording, true, nil
}

func (s *fakeRecordingStore) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	if s.updateErr != nil {
		return domain.Recording{}, s.updateErr
	}
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt != nil {
		return domain.Recording{}, errors.New("recording not found")
	}
	recording.Status = input.Status
	recording.FailureReason = ""
	if input.Status == domain.RecordingStatusFailed {
		recording.FailureReason = input.FailureReason
	}
	s.stored[input.ID] = recording
	return recording, nil
}

func (s *fakeRecordingStore) ResetForRetry(input recordings.RetryRecordingInput) (domain.Recording, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt != nil || recording.Status != domain.RecordingStatusFailed {
		return domain.Recording{}, errors.New("recording not found or not failed")
	}
	recording.Status = domain.RecordingStatusUploaded
	recording.FailureReason = ""
	recording.FailedAt = nil
	recording.CompletedAt = nil
	s.stored[input.ID] = recording
	return recording, nil
}

func (s *fakeRecordingStore) SoftDeleteForWorkspace(input recordings.SoftDeleteRecordingInput) (domain.Recording, bool, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt != nil {
		return domain.Recording{}, false, nil
	}
	now := time.Now().UTC()
	recording.DeletedAt = &now
	recording.DeletedByUserID = input.DeletedByUserID
	recording.UpdatedAt = now
	s.stored[input.ID] = recording
	return recording, true, nil
}

func (s *fakeRecordingStore) ListDeletedByWorkspace(input recordings.ListDeletedRecordingsInput) ([]domain.Recording, error) {
	result := []domain.Recording{}
	for _, recording := range s.stored {
		if recording.WorkspaceID == input.WorkspaceID && recording.DeletedAt != nil {
			result = append(result, recording)
		}
	}
	return result, nil
}

func (s *fakeRecordingStore) RestoreForWorkspace(input recordings.RestoreRecordingInput) (domain.Recording, bool, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt == nil {
		return domain.Recording{}, false, nil
	}
	recording.DeletedAt = nil
	recording.DeletedByUserID = ""
	recording.UpdatedAt = time.Now().UTC()
	s.stored[input.ID] = recording
	return recording, true, nil
}

func (s *fakeRecordingStore) PurgeForWorkspace(input recordings.PurgeRecordingInput) (recordings.PurgeRecordingResult, bool, error) {
	recording, ok := s.stored[input.ID]
	if !ok || recording.WorkspaceID != input.WorkspaceID || recording.DeletedAt == nil {
		return recordings.PurgeRecordingResult{}, false, nil
	}
	result := recordings.PurgeRecordingResult{Artifacts: []recordings.RecordingPurgeArtifact{}}
	appendArtifact := func(kind string, objectKey string) {
		if strings.TrimSpace(objectKey) == "" {
			return
		}
		artifact := recordings.RecordingPurgeArtifact{
			ID:           "rpa_" + recording.ID + "_" + kind,
			RecordingID:  recording.ID,
			WorkspaceID:  recording.WorkspaceID,
			ObjectKey:    objectKey,
			ArtifactKind: kind,
			Status:       recordings.RecordingPurgeArtifactStatusPending,
		}
		s.purgeArtifacts[artifact.ID] = artifact
		result.Artifacts = append(result.Artifacts, artifact)
	}
	appendArtifact(recordings.RecordingPurgeArtifactKindOriginalAudio, recording.AudioObjectKey)
	appendArtifact(recordings.RecordingPurgeArtifactKindNormalizedAudio, s.normalizedObjectKeys[recording.ID])
	delete(s.stored, recording.ID)
	delete(s.details, recording.ID)
	delete(s.normalizedObjectKeys, recording.ID)
	return result, true, nil
}

func (s *fakeRecordingStore) MarkPurgeArtifactDeleted(input recordings.MarkPurgeArtifactDeletedInput) (bool, error) {
	artifact, ok := s.purgeArtifacts[input.ID]
	if !ok {
		return false, nil
	}
	now := time.Now().UTC()
	artifact.Status = recordings.RecordingPurgeArtifactStatusDeleted
	artifact.DeletedAt = &now
	artifact.UpdatedAt = now
	artifact.LastError = ""
	s.purgeArtifacts[input.ID] = artifact
	return true, nil
}

func (s *fakeRecordingStore) MarkPurgeArtifactFailed(input recordings.MarkPurgeArtifactFailedInput) (bool, error) {
	artifact, ok := s.purgeArtifacts[input.ID]
	if !ok {
		return false, nil
	}
	artifact.Status = recordings.RecordingPurgeArtifactStatusFailed
	artifact.AttemptCount++
	artifact.LastError = input.LastError
	artifact.NextAttemptAt = input.NextAttemptAt
	artifact.UpdatedAt = time.Now().UTC()
	s.purgeArtifacts[input.ID] = artifact
	return true, nil
}

func (s *fakeRecordingStore) put(recording domain.Recording) {
	if recording.WorkspaceID == "" {
		recording.WorkspaceID = testWorkspaceID
	}
	s.stored[recording.ID] = recording
}

func (s *fakeWorkspaceStore) GetUser(_ context.Context, userID string) (domain.User, bool, error) {
	if s.getUserErr != nil {
		return domain.User{}, false, s.getUserErr
	}
	if s.user.ID != userID {
		return domain.User{}, false, nil
	}
	return s.user, true, nil
}

func (s *fakeWorkspaceStore) ListWorkspacesForUser(_ context.Context, userID string) ([]domain.WorkspaceWithRole, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if userID != s.user.ID {
		return []domain.WorkspaceWithRole{}, nil
	}
	return append([]domain.WorkspaceWithRole(nil), s.workspaces...), nil
}

func (s *fakeWorkspaceStore) GetWorkspaceForUser(_ context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error) {
	if s.getErr != nil {
		return domain.WorkspaceWithRole{}, false, s.getErr
	}
	workspace, ok := s.memberships[userID+":"+workspaceID]
	return workspace, ok, nil
}

func (s *fakeRecordingStore) GetTranscript(recordingID string) (recordings.RecordingTranscript, bool, error) {
	fixture, ok := s.details[recordingID]
	if fixture.transcriptErr != nil {
		return recordings.RecordingTranscript{}, false, fixture.transcriptErr
	}
	if !ok || !fixture.hasTranscript {
		return recordings.RecordingTranscript{}, false, nil
	}
	return fixture.transcript, true, nil
}

func (s *fakeRecordingStore) ListTranscriptSegments(recordingID string) ([]recordings.RecordingTranscriptSegment, error) {
	fixture := s.details[recordingID]
	if fixture.segmentsErr != nil {
		return nil, fixture.segmentsErr
	}
	return append([]recordings.RecordingTranscriptSegment(nil), fixture.segments...), nil
}

func (s *fakeRecordingStore) GetSummary(recordingID string) (recordings.RecordingSummary, bool, error) {
	fixture, ok := s.details[recordingID]
	if fixture.summaryErr != nil {
		return recordings.RecordingSummary{}, false, fixture.summaryErr
	}
	if !ok || !fixture.hasSummary {
		return recordings.RecordingSummary{}, false, nil
	}
	return fixture.summary, true, nil
}

func (s *fakeRecordingStore) GetMindMap(recordingID string) (recordings.RecordingMindMap, bool, error) {
	fixture, ok := s.details[recordingID]
	if fixture.mindMapErr != nil {
		return recordings.RecordingMindMap{}, false, fixture.mindMapErr
	}
	if !ok || !fixture.hasMindMap {
		return recordings.RecordingMindMap{}, false, nil
	}
	return fixture.mindMap, true, nil
}

func (s getErrRecordingStore) Create(recordings.CreateRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, errors.New("create should not be called")
}

func (s getErrRecordingStore) Get(string) (domain.Recording, bool, error) {
	return domain.Recording{}, false, s.err
}

func (s getErrRecordingStore) GetForWorkspace(recordings.GetRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, s.err
}

func (s getErrRecordingStore) ListByWorkspace(recordings.ListRecordingsInput) ([]domain.Recording, error) {
	return nil, s.err
}

func (s getErrRecordingStore) UpdateForWorkspace(recordings.UpdateRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, s.err
}

func (s getErrRecordingStore) GetTranscript(string) (recordings.RecordingTranscript, bool, error) {
	return recordings.RecordingTranscript{}, false, s.err
}

func (s getErrRecordingStore) ListTranscriptSegments(string) ([]recordings.RecordingTranscriptSegment, error) {
	return nil, s.err
}

func (s getErrRecordingStore) GetSummary(string) (recordings.RecordingSummary, bool, error) {
	return recordings.RecordingSummary{}, false, s.err
}

func (s getErrRecordingStore) GetMindMap(string) (recordings.RecordingMindMap, bool, error) {
	return recordings.RecordingMindMap{}, false, s.err
}

func (s getErrRecordingStore) ResetForRetry(recordings.RetryRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, s.err
}

func (s getErrRecordingStore) UpdateStatus(recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	return domain.Recording{}, s.err
}

func (s getErrRecordingStore) SoftDeleteForWorkspace(recordings.SoftDeleteRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, s.err
}

func (s getErrRecordingStore) ListDeletedByWorkspace(recordings.ListDeletedRecordingsInput) ([]domain.Recording, error) {
	return nil, s.err
}

func (s getErrRecordingStore) RestoreForWorkspace(recordings.RestoreRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, s.err
}

func (s getErrRecordingStore) PurgeForWorkspace(recordings.PurgeRecordingInput) (recordings.PurgeRecordingResult, bool, error) {
	return recordings.PurgeRecordingResult{}, false, s.err
}

func (s getErrRecordingStore) MarkPurgeArtifactDeleted(recordings.MarkPurgeArtifactDeletedInput) (bool, error) {
	return false, s.err
}

func (s getErrRecordingStore) MarkPurgeArtifactFailed(recordings.MarkPurgeArtifactFailedInput) (bool, error) {
	return false, s.err
}

func (s *recordingProcessorSpy) Enqueue(ctx context.Context, recording domain.Recording) error {
	s.contexts = append(s.contexts, ctx)
	s.contextErrs = append(s.contextErrs, ctx.Err())
	s.enqueued = append(s.enqueued, recording)
	return s.err
}

func (s *objectStoreSpy) PutObject(ctx context.Context, input storage.PutObjectInput) (storage.PutObjectResult, error) {
	if s.err != nil {
		return storage.PutObjectResult{}, s.err
	}
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return storage.PutObjectResult{}, err
	}
	s.puts = append(s.puts, storedObject{
		key:         input.Key,
		contentType: input.ContentType,
		body:        string(body),
	})
	return storage.PutObjectResult{Key: input.Key, SizeBytes: int64(len(body))}, nil
}

func (s *objectStoreSpy) GetObject(_ context.Context, key string) (storage.GetObjectResult, error) {
	for _, put := range s.puts {
		if put.key == key {
			return storage.GetObjectResult{
				Key:         key,
				Body:        io.NopCloser(strings.NewReader(put.body)),
				ContentType: put.contentType,
				SizeBytes:   int64(len(put.body)),
			}, nil
		}
	}
	return storage.GetObjectResult{Key: key, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (s *objectStoreSpy) PresignGetObject(_ context.Context, key string, ttl time.Duration) (string, error) {
	return "", nil
}

func (s *objectStoreSpy) DeleteObject(_ context.Context, key string) error {
	s.deletes = append(s.deletes, key)
	return s.deleteErr
}

func sameStringSet(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, value := range a {
		counts[value]++
	}
	for _, value := range b {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	return true
}

func TestGetMeReturnsCurrentUser(t *testing.T) {
	store := newFakeRecordingStore()
	workspaceStore := newFakeWorkspaceStore()
	router := NewRouterWithIdentity(store, workspaceStore, NewDevAuthResolver(testUserID), nil)

	request := httptest.NewRequest(http.MethodGet, "/me", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body domain.User
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.ID != testUserID || body.Email != "dev@local.soniq" || body.DisplayName != "Local Developer" {
		t.Fatalf("user = %+v, want dev user", body)
	}
}

func TestListWorkspacesReturnsCurrentUserMemberships(t *testing.T) {
	store := newFakeRecordingStore()
	workspaceStore := newFakeWorkspaceStore()
	router := NewRouterWithIdentity(store, workspaceStore, NewDevAuthResolver(testUserID), nil)

	request := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body listWorkspacesResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(body.Workspaces) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(body.Workspaces))
	}
	if body.Workspaces[0].ID != testWorkspaceID || body.Workspaces[0].Role != domain.WorkspaceRoleOwner {
		t.Fatalf("workspace = %+v, want default owner workspace", body.Workspaces[0])
	}
}

func TestListRecordingsReturnsOnlyWorkspaceRecordings(t *testing.T) {
	store := newFakeRecordingStore()
	store.put(domain.Recording{ID: "rec_default", WorkspaceID: testWorkspaceID, Title: "Default", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting})
	store.put(domain.Recording{ID: "rec_other", WorkspaceID: "wsp_other", Title: "Other", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting})
	router := NewRouterWithStore(store)

	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body listRecordingsResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(body.Recordings) != 1 {
		t.Fatalf("recordings = %d, want 1", len(body.Recordings))
	}
	if body.Recordings[0].ID != "rec_default" || body.Recordings[0].WorkspaceID != testWorkspaceID {
		t.Fatalf("recording = %+v, want default workspace recording", body.Recordings[0])
	}
}

func TestWorkspaceScopedRoutesReturnNotFoundForNonMember(t *testing.T) {
	store := newFakeRecordingStore()
	workspaceStore := newFakeWorkspaceStore()
	workspaceStore.memberships = map[string]domain.WorkspaceWithRole{}
	router := NewRouterWithIdentity(store, workspaceStore, NewDevAuthResolver(testUserID), nil)

	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestNewRouterWithStoreAcceptsRecordingStoreInterface(t *testing.T) {
	store := newFakeRecordingStore()
	router := NewRouterWithStore(store)

	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings", strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got, want := len(store.created), 1; got != want {
		t.Fatalf("store created calls = %d, want %d", got, want)
	}
	if store.created[0].WorkspaceID != testWorkspaceID {
		t.Fatalf("created workspace id = %q, want %q", store.created[0].WorkspaceID, testWorkspaceID)
	}
}

func TestGetRecordingUsesRecordingStoreInterface(t *testing.T) {
	store := newFakeRecordingStore()
	store.put(domain.Recording{
		ID:           "rec_fake",
		Title:        "Stored recording",
		Status:       domain.RecordingStatusUploaded,
		WorkflowType: domain.WorkflowTypeMemo,
		Language:     "en",
	})
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/rec_fake", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
}

func TestCreateRecordingReturnsCreatedRecording(t *testing.T) {
	store := newFakeRecordingStore()
	router := NewRouterWithStore(store)

	requestBody := strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`)
	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings", requestBody)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	contentType := response.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var body domain.Recording
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if !strings.HasPrefix(body.ID, "rec_") {
		t.Fatalf("id = %q, want rec_ prefix", body.ID)
	}
	if body.Title != "Weekly sync" {
		t.Fatalf("title = %q, want Weekly sync", body.Title)
	}
	if body.Status != domain.RecordingStatusUploaded {
		t.Fatalf("status = %q, want uploaded", body.Status)
	}
	if body.WorkflowType != domain.WorkflowTypeMeeting {
		t.Fatalf("workflow_type = %q, want meeting", body.WorkflowType)
	}
	if body.Language != "en" {
		t.Fatalf("language = %q, want en", body.Language)
	}

	stored, ok, err := store.Get(body.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error: %v", body.ID, err)
	}
	if !ok {
		t.Fatalf("store.Get(%q) ok = false, want true", body.ID)
	}
	if stored != body {
		t.Fatalf("stored recording = %+v, want response body %+v", stored, body)
	}
}

func TestCreateRecordingDoesNotEnqueueProcessingWithoutAudio(t *testing.T) {
	store := newFakeRecordingStore()
	processor := &recordingProcessorSpy{}
	router := NewRouterWithProcessor(store, processor)

	requestBody := strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`)
	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings", requestBody)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got, want := len(processor.enqueued), 0; got != want {
		t.Fatalf("enqueued recordings = %d, want %d", got, want)
	}
	if got, want := len(store.created), 1; got != want {
		t.Fatalf("created recordings = %d, want %d", got, want)
	}
}

func TestCreateRecordingDoesNotEnqueueInvalidRequest(t *testing.T) {
	store := newFakeRecordingStore()
	processor := &recordingProcessorSpy{}
	router := NewRouterWithProcessor(store, processor)

	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings", strings.NewReader(`{"title":"Podcast","workflow_type":"podcast","language":"en"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if got, want := len(processor.enqueued), 0; got != want {
		t.Fatalf("enqueued recordings = %d, want %d", got, want)
	}
}

func TestUploadRecordingStoresAudioCreatesRecordingAndEnqueues(t *testing.T) {
	store := newFakeRecordingStore()
	objectStore := &objectStoreSpy{}
	processor := &recordingProcessorSpy{}
	router := NewRouterWithStorage(store, processor, objectStore)

	request := newMultipartUploadRequest(t, "/workspaces/wsp_default/recordings/upload", map[string]string{
		"title":         "Weekly sync",
		"workflow_type": "meeting",
		"language":      "en",
	}, "audio", "weekly.wav", "audio/wav", "audio-bytes")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got, want := len(objectStore.puts), 1; got != want {
		t.Fatalf("object store puts = %d, want %d", got, want)
	}
	if objectStore.puts[0].contentType != "audio/wav" {
		t.Fatalf("object content type = %q, want audio/wav", objectStore.puts[0].contentType)
	}
	if objectStore.puts[0].body != "audio-bytes" {
		t.Fatalf("object body = %q, want audio-bytes", objectStore.puts[0].body)
	}
	if got, want := len(store.created), 1; got != want {
		t.Fatalf("store created calls = %d, want %d", got, want)
	}
	created := store.created[0]
	if created.Title != "Weekly sync" || created.WorkflowType != domain.WorkflowTypeMeeting || created.Language != "en" {
		t.Fatalf("created input = %+v, want upload metadata", created)
	}
	if created.WorkspaceID != testWorkspaceID {
		t.Fatalf("created WorkspaceID = %q, want %q", created.WorkspaceID, testWorkspaceID)
	}
	if created.AudioObjectKey == "" {
		t.Fatal("created AudioObjectKey is empty, want stored object key")
	}
	if got, want := objectStore.puts[0].key, created.AudioObjectKey; got != want {
		t.Fatalf("stored object key = %q, want created audio object key %q", got, want)
	}
	if !strings.HasPrefix(created.AudioObjectKey, "workspaces/wsp_default/recordings/") {
		t.Fatalf("created AudioObjectKey = %q, want workspace-scoped object key", created.AudioObjectKey)
	}
	if !strings.HasSuffix(created.AudioObjectKey, "/weekly.wav") {
		t.Fatalf("created AudioObjectKey = %q, want original filename suffix", created.AudioObjectKey)
	}
	if created.AudioContentType != "audio/wav" {
		t.Fatalf("created AudioContentType = %q, want audio/wav", created.AudioContentType)
	}
	if created.AudioSizeBytes != int64(len("audio-bytes")) {
		t.Fatalf("created AudioSizeBytes = %d, want %d", created.AudioSizeBytes, len("audio-bytes"))
	}
	if got, want := len(processor.enqueued), 1; got != want {
		t.Fatalf("enqueued recordings = %d, want %d", got, want)
	}
	if processor.enqueued[0].AudioObjectKey != created.AudioObjectKey {
		t.Fatalf("enqueued AudioObjectKey = %q, want %q", processor.enqueued[0].AudioObjectKey, created.AudioObjectKey)
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
	if body.Recording.AudioObjectKey != created.AudioObjectKey || body.Recording.AudioContentType != "audio/wav" || body.Recording.AudioSizeBytes != int64(len("audio-bytes")) {
		t.Fatalf("response audio metadata = %+v, want created audio metadata %+v", body.Recording, created)
	}
}

func TestUploadRecordingEnqueueContextKeepsValuesButIgnoresClientCancellation(t *testing.T) {
	store := newFakeRecordingStore()
	objectStore := &objectStoreSpy{}
	processor := &recordingProcessorSpy{}
	router := NewRouterWithStorage(store, processor, objectStore)

	request := newMultipartUploadRequest(t, "/workspaces/wsp_default/recordings/upload", map[string]string{
		"title":         "Weekly sync",
		"workflow_type": "meeting",
		"language":      "en",
	}, "audio", "weekly.wav", "audio/wav", "audio-bytes")
	ctx, cancel := context.WithCancel(context.WithValue(request.Context(), recordingRequestContextKey{}, "trace-context"))
	cancel()
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got, want := len(processor.contexts), 1; got != want {
		t.Fatalf("enqueue contexts = %d, want %d", got, want)
	}
	enqueueCtx := processor.contexts[0]
	if err := processor.contextErrs[0]; err != nil {
		t.Fatalf("enqueue context was already canceled during Enqueue: %v", err)
	}
	if got := enqueueCtx.Value(recordingRequestContextKey{}); got != "trace-context" {
		t.Fatalf("enqueue context value = %v, want trace-context", got)
	}
}

func TestRecordingAudioObjectKeyUsesWorkspaceScopedPrefix(t *testing.T) {
	key := recordingAudioObjectKey("wsp_default", "weekly.wav")

	if !strings.HasPrefix(key, "workspaces/wsp_default/recordings/") {
		t.Fatalf("recordingAudioObjectKey = %q, want workspace-scoped prefix", key)
	}
	if !strings.HasSuffix(key, "/weekly.wav") {
		t.Fatalf("recordingAudioObjectKey = %q, want original filename suffix", key)
	}
}

func TestRecordingAudioObjectKeyFallsBackToSafeFilename(t *testing.T) {
	key := recordingAudioObjectKey("wsp_default", "..")

	if !strings.HasPrefix(key, "workspaces/wsp_default/recordings/") {
		t.Fatalf("recordingAudioObjectKey = %q, want workspace-scoped prefix", key)
	}
	if !strings.HasSuffix(key, "/audio") {
		t.Fatalf("recordingAudioObjectKey = %q, want safe fallback filename", key)
	}
}

func TestUploadRecordingRequiresAudioFile(t *testing.T) {
	store := newFakeRecordingStore()
	objectStore := &objectStoreSpy{}
	processor := &recordingProcessorSpy{}
	router := NewRouterWithStorage(store, processor, objectStore)

	request := newMultipartUploadRequest(t, "/workspaces/wsp_default/recordings/upload", map[string]string{
		"title":         "Weekly sync",
		"workflow_type": "meeting",
		"language":      "en",
	}, "", "", "", "")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if len(objectStore.puts) != 0 || len(store.created) != 0 || len(processor.enqueued) != 0 {
		t.Fatalf("side effects = puts:%d created:%d enqueued:%d, want none", len(objectStore.puts), len(store.created), len(processor.enqueued))
	}
}

func TestUploadRecordingReturnsServerErrorWhenStorageFails(t *testing.T) {
	store := newFakeRecordingStore()
	objectStore := &objectStoreSpy{err: errObjectStoreFailed}
	processor := &recordingProcessorSpy{}
	router := NewRouterWithStorage(store, processor, objectStore)

	request := newMultipartUploadRequest(t, "/workspaces/wsp_default/recordings/upload", map[string]string{
		"title":         "Weekly sync",
		"workflow_type": "meeting",
		"language":      "en",
	}, "audio", "weekly.wav", "audio/wav", "audio-bytes")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if len(store.created) != 0 || len(processor.enqueued) != 0 {
		t.Fatalf("side effects = created:%d enqueued:%d, want none", len(store.created), len(processor.enqueued))
	}
}

func TestUploadRecordingDeletesStoredAudioWhenCreateFails(t *testing.T) {
	store := newFakeRecordingStore()
	store.createErr = errors.New("create recording failed")
	objectStore := &objectStoreSpy{}
	processor := &recordingProcessorSpy{}
	router := NewRouterWithStorage(store, processor, objectStore)

	request := newMultipartUploadRequest(t, "/workspaces/wsp_default/recordings/upload", map[string]string{
		"title":         "Weekly sync",
		"workflow_type": "meeting",
		"language":      "en",
	}, "audio", "weekly.wav", "audio/wav", "audio-bytes")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if got, want := len(objectStore.puts), 1; got != want {
		t.Fatalf("object store puts = %d, want %d", got, want)
	}
	if got, want := len(objectStore.deletes), 1; got != want {
		t.Fatalf("object store deletes = %d, want %d", got, want)
	}
	if objectStore.deletes[0] != objectStore.puts[0].key {
		t.Fatalf("deleted key = %q, want stored key %q", objectStore.deletes[0], objectStore.puts[0].key)
	}
	if len(processor.enqueued) != 0 {
		t.Fatalf("enqueued recordings = %d, want none", len(processor.enqueued))
	}
}

func TestUploadRecordingReturnsCreatedWhenEnqueueFails(t *testing.T) {
	store := newFakeRecordingStore()
	objectStore := &objectStoreSpy{}
	processor := &recordingProcessorSpy{err: errRecordingProcessorFailed}
	router := NewRouterWithStorage(store, processor, objectStore)

	request := newMultipartUploadRequest(t, "/workspaces/wsp_default/recordings/upload", map[string]string{
		"title":         "Weekly sync",
		"workflow_type": "meeting",
		"language":      "en",
	}, "audio", "weekly.wav", "audio/wav", "audio-bytes")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got, want := len(objectStore.puts), 1; got != want {
		t.Fatalf("object store puts = %d, want %d", got, want)
	}
	if len(objectStore.deletes) != 0 {
		t.Fatalf("object store deletes = %d, want none", len(objectStore.deletes))
	}
	if got, want := len(store.created), 1; got != want {
		t.Fatalf("store created calls = %d, want %d", got, want)
	}
	if got, want := len(processor.enqueued), 1; got != want {
		t.Fatalf("enqueued recordings = %d, want %d", got, want)
	}

	var body struct {
		Recording          domain.Recording `json:"recording"`
		ProcessingEnqueued bool             `json:"processing_enqueued"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.ProcessingEnqueued {
		t.Fatal("processing_enqueued = true, want false")
	}
	if body.Recording.ID == "" || body.Recording.AudioObjectKey == "" {
		t.Fatalf("body = %+v, want created recording with audio metadata", body)
	}
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
	if fileField != "" {
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
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestGetRecordingReturnsExistingRecording(t *testing.T) {
	store := newFakeRecordingStore()
	created, err := store.Create(recordings.CreateRecordingInput{
		WorkspaceID:  testWorkspaceID,
		Title:        "Lecture 1",
		WorkflowType: domain.WorkflowTypeLecture,
		Language:     "zh",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/"+created.ID, nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	contentType := response.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var body domain.Recording
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body != created {
		t.Fatalf("body = %+v, want %+v", body, created)
	}
}

func TestUpdateRecordingRenamesExistingRecording(t *testing.T) {
	store := newFakeRecordingStore()
	created, err := store.Create(recordings.CreateRecordingInput{
		WorkspaceID:  testWorkspaceID,
		Title:        "Weekly sync",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodPatch, "/workspaces/wsp_default/recordings/"+created.ID, strings.NewReader(`{"title":" Customer interview "}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body recordingResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.ID != created.ID || body.Title != "Customer interview" {
		t.Fatalf("updated recording = %+v, want renamed %s", body, created.ID)
	}
	stored := store.stored[created.ID]
	if stored.Title != "Customer interview" {
		t.Fatalf("stored title = %q, want Customer interview", stored.Title)
	}
}

func TestUpdateRecordingRejectsBlankTitle(t *testing.T) {
	store := newFakeRecordingStore()
	created, err := store.Create(recordings.CreateRecordingInput{
		WorkspaceID:  testWorkspaceID,
		Title:        "Weekly sync",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodPatch, "/workspaces/wsp_default/recordings/"+created.ID, strings.NewReader(`{"title":"   "}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if store.stored[created.ID].Title != "Weekly sync" {
		t.Fatalf("stored title = %q, want unchanged Weekly sync", store.stored[created.ID].Title)
	}
}

func TestUpdateRecordingReturnsNotFoundForMissingOrCrossWorkspaceRecording(t *testing.T) {
	store := newFakeRecordingStore()
	store.put(domain.Recording{ID: "rec_other", WorkspaceID: "wsp_other", Title: "Other", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting})
	router := NewRouterWithStore(store)

	for _, id := range []string{"rec_missing", "rec_other"} {
		request := httptest.NewRequest(http.MethodPatch, "/workspaces/wsp_default/recordings/"+id, strings.NewReader(`{"title":"Renamed"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Fatalf("update %s status code = %d, want %d", id, response.Code, http.StatusNotFound)
		}
	}
}

func TestDeleteRecordingSoftDeletesAndHidesRecording(t *testing.T) {
	store := newFakeRecordingStore()
	created, err := store.Create(recordings.CreateRecordingInput{
		WorkspaceID:  testWorkspaceID,
		Title:        "Weekly sync",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	router := NewRouterWithStore(store)

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/workspaces/wsp_default/recordings/"+created.ID, nil)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)

	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status code = %d, want %d; body=%s", deleteResponse.Code, http.StatusNoContent, deleteResponse.Body.String())
	}
	stored := store.stored[created.ID]
	if stored.DeletedAt == nil {
		t.Fatal("stored DeletedAt = nil, want soft-deleted recording")
	}
	if stored.DeletedByUserID != testUserID {
		t.Fatalf("stored DeletedByUserID = %q, want %q", stored.DeletedByUserID, testUserID)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/"+created.ID, nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusNotFound {
		t.Fatalf("get deleted status code = %d, want %d", getResponse.Code, http.StatusNotFound)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings", nil)
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status code = %d, want %d; body=%s", listResponse.Code, http.StatusOK, listResponse.Body.String())
	}
	var body listRecordingsResponse
	if err := json.NewDecoder(listResponse.Body).Decode(&body); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	if len(body.Recordings) != 0 {
		t.Fatalf("recordings = %+v, want deleted recording hidden", body.Recordings)
	}
}

func TestDeleteRecordingReturnsNotFoundForMissingOrCrossWorkspaceRecording(t *testing.T) {
	store := newFakeRecordingStore()
	store.put(domain.Recording{ID: "rec_other", WorkspaceID: "wsp_other", Title: "Other", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting})
	router := NewRouterWithStore(store)

	for _, id := range []string{"rec_missing", "rec_other"} {
		request := httptest.NewRequest(http.MethodDelete, "/workspaces/wsp_default/recordings/"+id, nil)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Fatalf("delete %s status code = %d, want %d", id, response.Code, http.StatusNotFound)
		}
	}
}

func TestListDeletedRecordingsAndRestoreRecording(t *testing.T) {
	store := newFakeRecordingStore()
	created, err := store.Create(recordings.CreateRecordingInput{
		WorkspaceID:  testWorkspaceID,
		Title:        "Weekly sync",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	router := NewRouterWithStore(store)

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/workspaces/wsp_default/recordings/"+created.ID, nil)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status code = %d, want %d; body=%s", deleteResponse.Code, http.StatusNoContent, deleteResponse.Body.String())
	}

	trashRequest := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/trash", nil)
	trashResponse := httptest.NewRecorder()
	router.ServeHTTP(trashResponse, trashRequest)
	if trashResponse.Code != http.StatusOK {
		t.Fatalf("trash status code = %d, want %d; body=%s", trashResponse.Code, http.StatusOK, trashResponse.Body.String())
	}
	var trashBody listRecordingsResponse
	if err := json.NewDecoder(trashResponse.Body).Decode(&trashBody); err != nil {
		t.Fatalf("decode trash body: %v", err)
	}
	if got, want := len(trashBody.Recordings), 1; got != want {
		t.Fatalf("trash recordings = %d, want %d", got, want)
	}
	if trashBody.Recordings[0].ID != created.ID || trashBody.Recordings[0].DeletedAt == nil {
		t.Fatalf("trash recording = %+v, want deleted recording %s", trashBody.Recordings[0], created.ID)
	}

	restoreRequest := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings/"+created.ID+"/restore", nil)
	restoreResponse := httptest.NewRecorder()
	router.ServeHTTP(restoreResponse, restoreRequest)
	if restoreResponse.Code != http.StatusOK {
		t.Fatalf("restore status code = %d, want %d; body=%s", restoreResponse.Code, http.StatusOK, restoreResponse.Body.String())
	}
	var restored recordingResponse
	if err := json.NewDecoder(restoreResponse.Body).Decode(&restored); err != nil {
		t.Fatalf("decode restore body: %v", err)
	}
	if restored.ID != created.ID {
		t.Fatalf("restored ID = %q, want %q", restored.ID, created.ID)
	}
	if restored.DeletedAt != nil || restored.DeletedByUserID != "" {
		t.Fatalf("restored deletion metadata = %v/%q, want cleared", restored.DeletedAt, restored.DeletedByUserID)
	}

	activeListRequest := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings", nil)
	activeListResponse := httptest.NewRecorder()
	router.ServeHTTP(activeListResponse, activeListRequest)
	if activeListResponse.Code != http.StatusOK {
		t.Fatalf("active list status code = %d, want %d; body=%s", activeListResponse.Code, http.StatusOK, activeListResponse.Body.String())
	}
	var activeBody listRecordingsResponse
	if err := json.NewDecoder(activeListResponse.Body).Decode(&activeBody); err != nil {
		t.Fatalf("decode active list body: %v", err)
	}
	if got, want := len(activeBody.Recordings), 1; got != want {
		t.Fatalf("active recordings = %d, want %d", got, want)
	}
	if activeBody.Recordings[0].ID != created.ID {
		t.Fatalf("active recording ID = %q, want %q", activeBody.Recordings[0].ID, created.ID)
	}

	trashAfterRestoreRequest := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/trash", nil)
	trashAfterRestoreResponse := httptest.NewRecorder()
	router.ServeHTTP(trashAfterRestoreResponse, trashAfterRestoreRequest)
	if trashAfterRestoreResponse.Code != http.StatusOK {
		t.Fatalf("trash after restore status code = %d, want %d", trashAfterRestoreResponse.Code, http.StatusOK)
	}
	trashBody = listRecordingsResponse{}
	if err := json.NewDecoder(trashAfterRestoreResponse.Body).Decode(&trashBody); err != nil {
		t.Fatalf("decode trash after restore body: %v", err)
	}
	if len(trashBody.Recordings) != 0 {
		t.Fatalf("trash recordings after restore = %+v, want empty", trashBody.Recordings)
	}
}

func TestRestoreRecordingReturnsNotFoundForMissingActiveOrCrossWorkspaceRecording(t *testing.T) {
	store := newFakeRecordingStore()
	deletedAt := time.Now().UTC()
	store.put(domain.Recording{ID: "rec_other", WorkspaceID: "wsp_other", Title: "Other", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting, DeletedAt: &deletedAt})
	store.put(domain.Recording{ID: "rec_active", WorkspaceID: testWorkspaceID, Title: "Active", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting})
	router := NewRouterWithStore(store)

	for _, id := range []string{"rec_missing", "rec_other", "rec_active"} {
		request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings/"+id+"/restore", nil)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Fatalf("restore %s status code = %d, want %d", id, response.Code, http.StatusNotFound)
		}
	}
}

func TestPurgeRecordingDeletesTrashRecordingAndArtifacts(t *testing.T) {
	store := newFakeRecordingStore()
	deletedAt := time.Now().UTC()
	store.put(domain.Recording{
		ID:             "rec_deleted",
		WorkspaceID:    testWorkspaceID,
		Title:          "Deleted",
		Status:         domain.RecordingStatusCompleted,
		WorkflowType:   domain.WorkflowTypeMeeting,
		AudioObjectKey: "workspaces/wsp_default/recordings/rec_deleted/original.wav",
		DeletedAt:      &deletedAt,
	})
	store.normalizedObjectKeys["rec_deleted"] = "workspaces/wsp_default/recordings/rec_deleted/normalized.wav"
	objectStore := &objectStoreSpy{}
	router := NewRouterWithStorage(store, noopRecordingProcessor{}, objectStore)

	request := httptest.NewRequest(http.MethodDelete, "/workspaces/wsp_default/recordings/rec_deleted/purge", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("purge status code = %d, want %d; body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if _, ok := store.stored["rec_deleted"]; ok {
		t.Fatal("recording still stored after purge")
	}
	if got, want := objectStore.deletes, []string{
		"workspaces/wsp_default/recordings/rec_deleted/original.wav",
		"workspaces/wsp_default/recordings/rec_deleted/normalized.wav",
	}; !sameStringSet(got, want) {
		t.Fatalf("deleted object keys = %+v, want %+v", got, want)
	}
	for id, artifact := range store.purgeArtifacts {
		if artifact.Status != recordings.RecordingPurgeArtifactStatusDeleted || artifact.DeletedAt == nil {
			t.Fatalf("artifact %s = %+v, want deleted", id, artifact)
		}
	}

	restoreRequest := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings/rec_deleted/restore", nil)
	restoreResponse := httptest.NewRecorder()
	router.ServeHTTP(restoreResponse, restoreRequest)
	if restoreResponse.Code != http.StatusNotFound {
		t.Fatalf("restore purged status code = %d, want %d", restoreResponse.Code, http.StatusNotFound)
	}
}

func TestPurgeRecordingReturnsNotFoundForMissingActiveOrCrossWorkspaceRecording(t *testing.T) {
	store := newFakeRecordingStore()
	deletedAt := time.Now().UTC()
	store.put(domain.Recording{ID: "rec_active", WorkspaceID: testWorkspaceID, Title: "Active", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting})
	store.put(domain.Recording{ID: "rec_other", WorkspaceID: "wsp_other", Title: "Other", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting, DeletedAt: &deletedAt})
	router := NewRouterWithStorage(store, noopRecordingProcessor{}, &objectStoreSpy{})

	for _, id := range []string{"rec_missing", "rec_active", "rec_other"} {
		request := httptest.NewRequest(http.MethodDelete, "/workspaces/wsp_default/recordings/"+id+"/purge", nil)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Fatalf("purge %s status code = %d, want %d", id, response.Code, http.StatusNotFound)
		}
	}
}

func TestPurgeRecordingRequiresObjectStorage(t *testing.T) {
	store := newFakeRecordingStore()
	deletedAt := time.Now().UTC()
	store.put(domain.Recording{ID: "rec_deleted", WorkspaceID: testWorkspaceID, Title: "Deleted", Status: domain.RecordingStatusCompleted, WorkflowType: domain.WorkflowTypeMeeting, DeletedAt: &deletedAt})
	router := NewRouterWithStore(store)

	request := httptest.NewRequest(http.MethodDelete, "/workspaces/wsp_default/recordings/rec_deleted/purge", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("purge status code = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if _, ok := store.stored["rec_deleted"]; !ok {
		t.Fatal("recording purged even though object storage is not configured")
	}
}

func TestPurgeRecordingKeepsFailedArtifactCleanupRetryable(t *testing.T) {
	store := newFakeRecordingStore()
	deletedAt := time.Now().UTC()
	store.put(domain.Recording{
		ID:             "rec_deleted",
		WorkspaceID:    testWorkspaceID,
		Title:          "Deleted",
		Status:         domain.RecordingStatusCompleted,
		WorkflowType:   domain.WorkflowTypeMeeting,
		AudioObjectKey: "workspaces/wsp_default/recordings/rec_deleted/original.wav",
		DeletedAt:      &deletedAt,
	})
	objectStore := &objectStoreSpy{deleteErr: errObjectStoreFailed}
	router := NewRouterWithStorage(store, noopRecordingProcessor{}, objectStore)

	request := httptest.NewRequest(http.MethodDelete, "/workspaces/wsp_default/recordings/rec_deleted/purge", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("purge status code = %d, want %d; body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if _, ok := store.stored["rec_deleted"]; ok {
		t.Fatal("recording still stored after purge")
	}
	for id, artifact := range store.purgeArtifacts {
		if artifact.Status != recordings.RecordingPurgeArtifactStatusFailed || artifact.LastError == "" || artifact.NextAttemptAt.IsZero() {
			t.Fatalf("artifact %s = %+v, want failed retryable cleanup", id, artifact)
		}
	}
}

func TestGetRecordingDetailsReturnsTranscriptSegmentsAndSummary(t *testing.T) {
	store := newFakeRecordingStore()
	recording := domain.Recording{
		ID:           "rec_details",
		Title:        "Weekly sync",
		Status:       domain.RecordingStatusCompleted,
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "zh",
	}
	store.put(recording)
	store.details[recording.ID] = recordingDetailsFixture{
		transcript: recordings.RecordingTranscript{
			RecordingID:   recording.ID,
			Provider:      "dashscope_asr",
			Model:         "paraformer-v2",
			Language:      "zh",
			Text:          "大家早上好。",
			RawResultJSON: []byte(`{"provider":"raw-transcript"}`),
		},
		segments: []recordings.RecordingTranscriptSegment{{
			RecordingID:  recording.ID,
			SegmentIndex: 0,
			StartMS:      90,
			EndMS:        1200,
			SpeakerLabel: "0",
			Text:         "大家早上好。",
			Confidence:   0.98,
		}},
		summary: recordings.RecordingSummary{
			RecordingID:     recording.ID,
			Provider:        "openai_compatible_llm",
			Model:           "qwen3.7-plus",
			Type:            domain.WorkflowTypeMeeting,
			Title:           "Weekly sync",
			Overview:        "同步了测试计划。",
			ContentMarkdown: "## 概览\n同步了测试计划。",
			RawResultJSON:   []byte(`{"provider":"raw-summary"}`),
		},
		mindMap: recordings.RecordingMindMap{
			RecordingID:     recording.ID,
			Provider:        "openai_compatible_llm",
			Model:           "qwen3.7-plus",
			Title:           "Weekly sync mind map",
			RootJSON:        []byte(`{"label":"Weekly sync","children":[{"label":"测试计划"}]}`),
			ContentMarkdown: "- Weekly sync\n  - 测试计划",
			RawResultJSON:   []byte(`{"provider":"raw-mind-map"}`),
		},
		hasTranscript: true,
		hasSummary:    true,
		hasMindMap:    true,
	}
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/"+recording.ID+"/details", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["Recording"] != nil || body["Transcript"] != nil || body["Summary"] != nil {
		t.Fatalf("body uses PascalCase persistence fields: %#v", body)
	}
	recordingBody, ok := body["recording"].(map[string]any)
	if !ok || recordingBody["id"] != recording.ID {
		t.Fatalf("recording = %#v, want id %q", body["recording"], recording.ID)
	}
	transcriptBody, ok := body["transcript"].(map[string]any)
	if !ok {
		t.Fatalf("transcript = %#v, want object", body["transcript"])
	}
	if transcriptBody["RecordingID"] != nil || transcriptBody["RawResultJSON"] != nil || transcriptBody["raw_result_json"] != nil {
		t.Fatalf("transcript leaked persistence/raw fields: %#v", transcriptBody)
	}
	if transcriptBody["recording_id"] != recording.ID || transcriptBody["provider"] != "dashscope_asr" || transcriptBody["text"] == "" {
		t.Fatalf("transcript = %#v, want public transcript DTO", transcriptBody)
	}
	segments, ok := body["segments"].([]any)
	if !ok || len(segments) != 1 {
		t.Fatalf("segments = %#v, want one segment", body["segments"])
	}
	segmentBody, ok := segments[0].(map[string]any)
	if !ok || segmentBody["segment_index"] == nil || segmentBody["speaker_label"] != "0" || segmentBody["text"] == "" {
		t.Fatalf("segment = %#v, want public segment DTO", segments[0])
	}
	summaryBody, ok := body["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary = %#v, want object", body["summary"])
	}
	if summaryBody["RecordingID"] != nil || summaryBody["RawResultJSON"] != nil || summaryBody["raw_result_json"] != nil {
		t.Fatalf("summary leaked persistence/raw fields: %#v", summaryBody)
	}
	if summaryBody["recording_id"] != recording.ID || summaryBody["provider"] != "openai_compatible_llm" || summaryBody["overview"] == "" {
		t.Fatalf("summary = %#v, want public summary DTO", summaryBody)
	}
	mindMapBody, ok := body["mind_map"].(map[string]any)
	if !ok {
		t.Fatalf("mind_map = %#v, want object", body["mind_map"])
	}
	if mindMapBody["RecordingID"] != nil || mindMapBody["RawResultJSON"] != nil || mindMapBody["raw_result_json"] != nil {
		t.Fatalf("mind map leaked persistence/raw fields: %#v", mindMapBody)
	}
	mindMapRoot, ok := mindMapBody["root"].(map[string]any)
	if !ok || mindMapRoot["label"] != "Weekly sync" {
		t.Fatalf("mind map root = %#v, want public root DTO", mindMapBody["root"])
	}
}

func TestGetRecordingDetailsReturnsInternalServerErrorWhenDetailsReadFails(t *testing.T) {
	tests := []struct {
		name    string
		details recordingDetailsFixture
	}{
		{
			name:    "transcript read fails",
			details: recordingDetailsFixture{transcriptErr: errRecordingDetailsFailed},
		},
		{
			name: "segments read fails",
			details: recordingDetailsFixture{
				transcript:    recordings.RecordingTranscript{RecordingID: "rec_details_error", Text: "hello"},
				hasTranscript: true,
				segmentsErr:   errRecordingDetailsFailed,
			},
		},
		{
			name:    "summary read fails",
			details: recordingDetailsFixture{summaryErr: errRecordingDetailsFailed},
		},
		{
			name:    "mind map read fails",
			details: recordingDetailsFixture{mindMapErr: errRecordingDetailsFailed},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeRecordingStore()
			recording := domain.Recording{
				ID:           "rec_details_error",
				Title:        "Stored recording",
				Status:       domain.RecordingStatusUploaded,
				WorkflowType: domain.WorkflowTypeMeeting,
				Language:     "zh",
			}
			store.put(recording)
			store.details[recording.ID] = tt.details
			router := NewRouterWithStore(store)
			request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/"+recording.ID+"/details", nil)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
			}
		})
	}
}

func TestGetRecordingReturnsNotFoundForUnknownRecording(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/rec_missing", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestGetRecordingReturnsServerErrorWhenStoreGetFails(t *testing.T) {
	router := NewRouterWithStore(getErrRecordingStore{err: errRecordingGetFailed})
	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/rec_db_error", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
}

func TestGetRecordingStatusReturnsExistingRecordingStatus(t *testing.T) {
	store := newFakeRecordingStore()
	created, err := store.Create(recordings.CreateRecordingInput{
		WorkspaceID:  testWorkspaceID,
		Title:        "Interview",
		WorkflowType: domain.WorkflowTypeInterview,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/"+created.ID+"/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	contentType := response.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var body struct {
		ID          string                 `json:"id"`
		WorkspaceID string                 `json:"workspace_id"`
		Status      domain.RecordingStatus `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.ID != created.ID {
		t.Fatalf("id = %q, want %q", body.ID, created.ID)
	}
	if body.WorkspaceID != testWorkspaceID {
		t.Fatalf("workspace_id = %q, want %q", body.WorkspaceID, testWorkspaceID)
	}
	if body.Status != domain.RecordingStatusUploaded {
		t.Fatalf("status = %q, want uploaded", body.Status)
	}
}

func TestGetRecordingStatusReturnsFailureMetadata(t *testing.T) {
	store := newFakeRecordingStore()
	store.put(domain.Recording{
		ID:            "rec_failed",
		WorkspaceID:   testWorkspaceID,
		Title:         "Failed recording",
		Status:        domain.RecordingStatusFailed,
		WorkflowType:  domain.WorkflowTypeMeeting,
		Language:      "en",
		FailureReason: "transcribe audio: provider failed",
	})
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/rec_failed/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		ID            string                 `json:"id"`
		WorkspaceID   string                 `json:"workspace_id"`
		Status        domain.RecordingStatus `json:"status"`
		FailureReason string                 `json:"failure_reason"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Status != domain.RecordingStatusFailed || body.FailureReason != "transcribe audio: provider failed" {
		t.Fatalf("status body = %+v, want failed reason", body)
	}
}

func TestRetryFailedRecordingResetsAndEnqueues(t *testing.T) {
	store := newFakeRecordingStore()
	store.put(domain.Recording{
		ID:             "rec_failed",
		WorkspaceID:    testWorkspaceID,
		Title:          "Failed recording",
		Status:         domain.RecordingStatusFailed,
		WorkflowType:   domain.WorkflowTypeMeeting,
		Language:       "en",
		AudioObjectKey: "workspaces/wsp_default/recordings/rec_failed/audio.wav",
		FailureReason:  "transcribe audio: provider failed",
	})
	processor := &recordingProcessorSpy{}
	router := NewRouterWithProcessor(store, processor)
	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings/rec_failed/retry", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body retryRecordingResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if !body.ProcessingEnqueued {
		t.Fatal("processing_enqueued = false, want true")
	}
	if body.Recording.Status != domain.RecordingStatusUploaded || body.Recording.FailureReason != "" {
		t.Fatalf("recording = %+v, want uploaded with cleared failure", body.Recording)
	}
	if got, want := len(processor.enqueued), 1; got != want {
		t.Fatalf("enqueued recordings = %d, want %d", got, want)
	}
	if processor.enqueued[0].Status != domain.RecordingStatusUploaded || processor.enqueued[0].FailureReason != "" {
		t.Fatalf("enqueued recording = %+v, want uploaded retry recording", processor.enqueued[0])
	}
}

func TestRetryRecordingRejectsNonFailedRecording(t *testing.T) {
	store := newFakeRecordingStore()
	store.put(domain.Recording{
		ID:             "rec_completed",
		WorkspaceID:    testWorkspaceID,
		Title:          "Completed recording",
		Status:         domain.RecordingStatusCompleted,
		WorkflowType:   domain.WorkflowTypeMeeting,
		Language:       "en",
		AudioObjectKey: "workspaces/wsp_default/recordings/rec_completed/audio.wav",
	})
	processor := &recordingProcessorSpy{}
	router := NewRouterWithProcessor(store, processor)
	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings/rec_completed/retry", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusConflict, response.Body.String())
	}
	if len(processor.enqueued) != 0 {
		t.Fatalf("enqueued recordings = %d, want none", len(processor.enqueued))
	}
}

func TestRetryRecordingMarksFailedWhenEnqueueFails(t *testing.T) {
	store := newFakeRecordingStore()
	store.put(domain.Recording{
		ID:             "rec_failed",
		WorkspaceID:    testWorkspaceID,
		Title:          "Failed recording",
		Status:         domain.RecordingStatusFailed,
		WorkflowType:   domain.WorkflowTypeMeeting,
		Language:       "en",
		AudioObjectKey: "workspaces/wsp_default/recordings/rec_failed/audio.wav",
		FailureReason:  "old failure",
	})
	processor := &recordingProcessorSpy{err: errRecordingProcessorFailed}
	router := NewRouterWithProcessor(store, processor)
	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings/rec_failed/retry", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body retryRecordingResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.ProcessingEnqueued {
		t.Fatal("processing_enqueued = true, want false")
	}
	if body.Recording.Status != domain.RecordingStatusFailed || !strings.Contains(body.Recording.FailureReason, "retry enqueue failed") {
		t.Fatalf("recording = %+v, want failed retry enqueue reason", body.Recording)
	}
}

func TestRetryRecordingReturnsServerErrorWhenFailureRestoreFails(t *testing.T) {
	store := newFakeRecordingStore()
	store.updateErr = errors.New("update failed")
	store.put(domain.Recording{
		ID:             "rec_failed",
		WorkspaceID:    testWorkspaceID,
		Title:          "Failed recording",
		Status:         domain.RecordingStatusFailed,
		WorkflowType:   domain.WorkflowTypeMeeting,
		Language:       "en",
		AudioObjectKey: "workspaces/wsp_default/recordings/rec_failed/audio.wav",
		FailureReason:  "old failure",
	})
	processor := &recordingProcessorSpy{err: errRecordingProcessorFailed}
	router := NewRouterWithProcessor(store, processor)
	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings/rec_failed/retry", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	if got, want := len(processor.enqueued), 1; got != want {
		t.Fatalf("enqueued recordings = %d, want %d", got, want)
	}
}

func TestGetRecordingStatusReturnsNotFoundForUnknownRecording(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_default/recordings/rec_missing/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCreateRecordingRejectsInvalidJSON(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings", bytes.NewBufferString(`{"title":`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRecordingRejectsInvalidWorkflowType(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodPost, "/workspaces/wsp_default/recordings", strings.NewReader(`{"title":"Podcast","workflow_type":"podcast","language":"en"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRecordingRejectsNonPOST(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodPut, "/workspaces/wsp_default/recordings", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
