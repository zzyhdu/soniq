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

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

var errRecordingProcessorFailed = errors.New("recording processor failed")
var errObjectStoreFailed = errors.New("object store failed")
var errRecordingGetFailed = errors.New("recording get failed")

type recordingProcessorSpy struct {
	enqueued []domain.Recording
	err      error
}

type fakeRecordingStore struct {
	created   []recordings.CreateRecordingInput
	stored    map[string]domain.Recording
	details   map[string]recordingDetailsFixture
	nextID    int
	createErr error
}

type recordingDetailsFixture struct {
	transcript    recordings.RecordingTranscript
	segments      []recordings.RecordingTranscriptSegment
	summary       recordings.RecordingSummary
	hasTranscript bool
	hasSummary    bool
}

type recordingOnlyStore struct {
	stored map[string]domain.Recording
}

type getErrRecordingStore struct {
	err error
}

type objectStoreSpy struct {
	puts    []storedObject
	deletes []string
	err     error
}

type storedObject struct {
	key         string
	contentType string
	body        string
}

func newFakeRecordingStore() *fakeRecordingStore {
	return &fakeRecordingStore{stored: make(map[string]domain.Recording), details: make(map[string]recordingDetailsFixture)}
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
	return recording, ok, nil
}

func (s *fakeRecordingStore) put(recording domain.Recording) {
	s.stored[recording.ID] = recording
}

func (s *fakeRecordingStore) GetTranscript(recordingID string) (recordings.RecordingTranscript, bool) {
	fixture, ok := s.details[recordingID]
	if !ok || !fixture.hasTranscript {
		return recordings.RecordingTranscript{}, false
	}
	return fixture.transcript, true
}

func (s *fakeRecordingStore) ListTranscriptSegments(recordingID string) []recordings.RecordingTranscriptSegment {
	fixture := s.details[recordingID]
	return append([]recordings.RecordingTranscriptSegment(nil), fixture.segments...)
}

func (s *fakeRecordingStore) GetSummary(recordingID string) (recordings.RecordingSummary, bool) {
	fixture, ok := s.details[recordingID]
	if !ok || !fixture.hasSummary {
		return recordings.RecordingSummary{}, false
	}
	return fixture.summary, true
}

func (s recordingOnlyStore) Create(recordings.CreateRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, errors.New("create should not be called")
}

func (s recordingOnlyStore) Get(id string) (domain.Recording, bool, error) {
	recording, ok := s.stored[id]
	return recording, ok, nil
}

func (s getErrRecordingStore) Create(recordings.CreateRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, errors.New("create should not be called")
}

func (s getErrRecordingStore) Get(string) (domain.Recording, bool, error) {
	return domain.Recording{}, false, s.err
}

func (s *recordingProcessorSpy) Enqueue(recording domain.Recording) error {
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

func (s *objectStoreSpy) DeleteObject(_ context.Context, key string) error {
	s.deletes = append(s.deletes, key)
	return nil
}

func TestNewRouterWithStoreAcceptsRecordingStoreInterface(t *testing.T) {
	store := newFakeRecordingStore()
	router := NewRouterWithStore(store)

	request := httptest.NewRequest(http.MethodPost, "/recordings", strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got, want := len(store.created), 1; got != want {
		t.Fatalf("store created calls = %d, want %d", got, want)
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
	request := httptest.NewRequest(http.MethodGet, "/recordings/rec_fake", nil)
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
	request := httptest.NewRequest(http.MethodPost, "/recordings", requestBody)
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

func TestUploadRecordingStoresAudioCreatesRecordingAndEnqueues(t *testing.T) {
	store := newFakeRecordingStore()
	objectStore := &objectStoreSpy{}
	processor := &recordingProcessorSpy{}
	router := NewRouterWithStorage(store, processor, objectStore)

	request := newMultipartUploadRequest(t, "/recordings/upload", map[string]string{
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
	if created.AudioObjectKey == "" {
		t.Fatal("created AudioObjectKey is empty, want stored object key")
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

func TestUploadRecordingRequiresAudioFile(t *testing.T) {
	store := newFakeRecordingStore()
	objectStore := &objectStoreSpy{}
	processor := &recordingProcessorSpy{}
	router := NewRouterWithStorage(store, processor, objectStore)

	request := newMultipartUploadRequest(t, "/recordings/upload", map[string]string{
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

	request := newMultipartUploadRequest(t, "/recordings/upload", map[string]string{
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

	request := newMultipartUploadRequest(t, "/recordings/upload", map[string]string{
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

	request := newMultipartUploadRequest(t, "/recordings/upload", map[string]string{
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
		hasTranscript: true,
		hasSummary:    true,
	}
	router := NewRouterWithStore(store)
	request := httptest.NewRequest(http.MethodGet, "/recordings/"+recording.ID+"/details", nil)
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
}

func TestGetRecordingDetailsReturnsInternalServerErrorWhenDetailsStoreMissing(t *testing.T) {
	recording := domain.Recording{
		ID:           "rec_no_details_store",
		Title:        "Stored recording",
		Status:       domain.RecordingStatusUploaded,
		WorkflowType: domain.WorkflowTypeMemo,
		Language:     "en",
	}
	router := NewRouterWithStore(recordingOnlyStore{stored: map[string]domain.Recording{recording.ID: recording}})
	request := httptest.NewRequest(http.MethodGet, "/recordings/"+recording.ID+"/details", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
}

func TestGetRecordingReturnsNotFoundForUnknownRecording(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodGet, "/recordings/rec_missing", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestGetRecordingReturnsServerErrorWhenStoreGetFails(t *testing.T) {
	router := NewRouterWithStore(getErrRecordingStore{err: errRecordingGetFailed})
	request := httptest.NewRequest(http.MethodGet, "/recordings/rec_db_error", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
}

func TestGetRecordingStatusReturnsExistingRecordingStatus(t *testing.T) {
	store := newFakeRecordingStore()
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
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodGet, "/recordings/rec_missing/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCreateRecordingRejectsInvalidJSON(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodPost, "/recordings", bytes.NewBufferString(`{"title":`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRecordingRejectsInvalidWorkflowType(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodPost, "/recordings", strings.NewReader(`{"title":"Podcast","workflow_type":"podcast","language":"en"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateRecordingRejectsNonPOST(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodGet, "/recordings", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
