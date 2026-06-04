package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
)

var errRecordingProcessorFailed = errors.New("recording processor failed")

type recordingProcessorSpy struct {
	enqueued []domain.Recording
	err      error
}

func (s *recordingProcessorSpy) Enqueue(recording domain.Recording) error {
	s.enqueued = append(s.enqueued, recording)
	return s.err
}

func TestCreateRecordingReturnsCreatedRecording(t *testing.T) {
	store := recordings.NewMemoryStore()
	router := NewRouterWithStore(store)

	requestBody := strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`)
	request := httptest.NewRequest(http.MethodPost, "/recordings", requestBody)
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

	stored, ok := store.Get(body.ID)
	if !ok {
		t.Fatalf("store.Get(%q) ok = false, want true", body.ID)
	}
	if stored != body {
		t.Fatalf("stored recording = %+v, want response body %+v", stored, body)
	}
}

func TestCreateRecordingEnqueuesProcessing(t *testing.T) {
	store := recordings.NewMemoryStore()
	processor := &recordingProcessorSpy{}
	router := NewRouterWithProcessor(store, processor)

	requestBody := strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`)
	request := httptest.NewRequest(http.MethodPost, "/recordings", requestBody)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got, want := len(processor.enqueued), 1; got != want {
		t.Fatalf("enqueued recordings = %d, want %d", got, want)
	}
	if !strings.HasPrefix(processor.enqueued[0].ID, "rec_") {
		t.Fatalf("enqueued id = %q, want rec_ prefix", processor.enqueued[0].ID)
	}
	if processor.enqueued[0].WorkflowType != domain.WorkflowTypeMeeting {
		t.Fatalf("enqueued workflow_type = %q, want meeting", processor.enqueued[0].WorkflowType)
	}
}

func TestCreateRecordingDoesNotEnqueueInvalidRequest(t *testing.T) {
	store := recordings.NewMemoryStore()
	processor := &recordingProcessorSpy{}
	router := NewRouterWithProcessor(store, processor)

	request := httptest.NewRequest(http.MethodPost, "/recordings", strings.NewReader(`{"title":"Podcast","workflow_type":"podcast","language":"en"}`))
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

func TestCreateRecordingReturnsServerErrorWhenEnqueueFails(t *testing.T) {
	store := recordings.NewMemoryStore()
	processor := &recordingProcessorSpy{err: errRecordingProcessorFailed}
	router := NewRouterWithProcessor(store, processor)

	requestBody := strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`)
	request := httptest.NewRequest(http.MethodPost, "/recordings", requestBody)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got, want := len(processor.enqueued), 1; got != want {
		t.Fatalf("enqueued recordings = %d, want %d", got, want)
	}
}

func TestGetRecordingReturnsExistingRecording(t *testing.T) {
	store := recordings.NewMemoryStore()
	created, err := store.Create(recordings.CreateRecordingInput{
		Title:        "Lecture 1",
		WorkflowType: domain.WorkflowTypeLecture,
		Language:     "zh",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodGet, "/recordings/"+created.ID, nil)
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

func TestGetRecordingReturnsNotFoundForUnknownRecording(t *testing.T) {
	router := NewRouterWithStore(recordings.NewMemoryStore())
	request := httptest.NewRequest(http.MethodGet, "/recordings/rec_missing", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestGetRecordingStatusReturnsExistingRecordingStatus(t *testing.T) {
	store := recordings.NewMemoryStore()
	created, err := store.Create(recordings.CreateRecordingInput{
		Title:        "Interview",
		WorkflowType: domain.WorkflowTypeInterview,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodGet, "/recordings/"+created.ID+"/status", nil)
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
		ID     string                 `json:"id"`
		Status domain.RecordingStatus `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.ID != created.ID {
		t.Fatalf("id = %q, want %q", body.ID, created.ID)
	}
	if body.Status != domain.RecordingStatusUploaded {
		t.Fatalf("status = %q, want uploaded", body.Status)
	}
}

func TestGetRecordingStatusReturnsNotFoundForUnknownRecording(t *testing.T) {
	router := NewRouterWithStore(recordings.NewMemoryStore())
	request := httptest.NewRequest(http.MethodGet, "/recordings/rec_missing/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCreateRecordingRejectsInvalidJSON(t *testing.T) {
	router := NewRouterWithStore(recordings.NewMemoryStore())
	request := httptest.NewRequest(http.MethodPost, "/recordings", bytes.NewBufferString(`{"title":`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRecordingRejectsInvalidWorkflowType(t *testing.T) {
	router := NewRouterWithStore(recordings.NewMemoryStore())
	request := httptest.NewRequest(http.MethodPost, "/recordings", strings.NewReader(`{"title":"Podcast","workflow_type":"podcast","language":"en"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRecordingRejectsNonPOST(t *testing.T) {
	router := NewRouterWithStore(recordings.NewMemoryStore())
	request := httptest.NewRequest(http.MethodGet, "/recordings", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
