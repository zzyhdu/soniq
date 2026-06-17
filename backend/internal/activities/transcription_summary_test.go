package activities

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

type transcriptionSummaryStoreSpy struct {
	recordings         map[string]domain.Recording
	normalizedAudio    recordings.RecordingNormalizedAudio
	normalizedAudioErr error
	transcript         recordings.RecordingTranscript
	transcriptErr      error
	transcripts        []recordings.UpsertTranscriptInput
	summaries          []recordings.UpsertSummaryInput
	mindMaps           []recordings.UpsertMindMapInput
}

func (s *transcriptionSummaryStoreSpy) Get(id string) (domain.Recording, bool, error) {
	if s.recordings == nil {
		return domain.Recording{}, false, nil
	}
	recording, ok := s.recordings[id]
	return recording, ok, nil
}

func (s *transcriptionSummaryStoreSpy) GetForWorkspace(input recordings.GetRecordingInput) (domain.Recording, bool, error) {
	recording, ok, err := s.Get(input.ID)
	if err != nil {
		return domain.Recording{}, false, err
	}
	if !ok || recording.WorkspaceID != input.WorkspaceID {
		return domain.Recording{}, false, nil
	}
	return recording, true, nil
}

func (s *transcriptionSummaryStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	recording, ok, err := s.Get(input.ID)
	if err != nil {
		return domain.Recording{}, err
	}
	if !ok {
		return domain.Recording{}, errors.New("recording not found")
	}
	recording.Status = input.Status
	return recording, nil
}

func (s *transcriptionSummaryStoreSpy) UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error) {
	return recordings.RecordingAudioProbe{RecordingID: input.RecordingID}, nil
}

func (s *transcriptionSummaryStoreSpy) UpsertNormalizedAudio(input recordings.UpsertNormalizedAudioInput) (recordings.RecordingNormalizedAudio, error) {
	s.normalizedAudio = recordings.RecordingNormalizedAudio{RecordingID: input.RecordingID, ObjectKey: input.ObjectKey}
	return s.normalizedAudio, nil
}

func (s *transcriptionSummaryStoreSpy) GetNormalizedAudio(recordingID string) (recordings.RecordingNormalizedAudio, bool, error) {
	if s.normalizedAudioErr != nil {
		return recordings.RecordingNormalizedAudio{}, false, s.normalizedAudioErr
	}
	if s.normalizedAudio.RecordingID == "" || s.normalizedAudio.RecordingID != recordingID {
		return recordings.RecordingNormalizedAudio{}, false, nil
	}
	return s.normalizedAudio, true, nil
}

func (s *transcriptionSummaryStoreSpy) UpsertTranscript(input recordings.UpsertTranscriptInput) (recordings.RecordingTranscript, error) {
	s.transcripts = append(s.transcripts, input)
	return recordings.RecordingTranscript{
		RecordingID:   input.RecordingID,
		Provider:      input.Provider,
		Model:         input.Model,
		Language:      input.Language,
		Text:          input.Text,
		RawResultJSON: append([]byte(nil), input.RawResultJSON...),
		TranscribedAt: input.TranscribedAt,
	}, nil
}

func (s *transcriptionSummaryStoreSpy) GetTranscript(recordingID string) (recordings.RecordingTranscript, bool, error) {
	if s.transcriptErr != nil {
		return recordings.RecordingTranscript{}, false, s.transcriptErr
	}
	if s.transcript.RecordingID == "" || s.transcript.RecordingID != recordingID {
		return recordings.RecordingTranscript{}, false, nil
	}
	return s.transcript, true, nil
}

func (s *transcriptionSummaryStoreSpy) UpsertSummary(input recordings.UpsertSummaryInput) (recordings.RecordingSummary, error) {
	s.summaries = append(s.summaries, input)
	return recordings.RecordingSummary{
		RecordingID:     input.RecordingID,
		Provider:        input.Provider,
		Model:           input.Model,
		Type:            input.Type,
		Title:           input.Title,
		Overview:        input.Overview,
		ContentMarkdown: input.ContentMarkdown,
		RawResultJSON:   append([]byte(nil), input.RawResultJSON...),
		SummarizedAt:    input.SummarizedAt,
	}, nil
}

func (s *transcriptionSummaryStoreSpy) GetSummary(recordingID string) (recordings.RecordingSummary, bool, error) {
	for i := len(s.summaries) - 1; i >= 0; i-- {
		summary := s.summaries[i]
		if summary.RecordingID == recordingID {
			return recordings.RecordingSummary{
				RecordingID:     summary.RecordingID,
				Provider:        summary.Provider,
				Model:           summary.Model,
				Type:            summary.Type,
				Title:           summary.Title,
				Overview:        summary.Overview,
				ContentMarkdown: summary.ContentMarkdown,
				RawResultJSON:   append([]byte(nil), summary.RawResultJSON...),
				SummarizedAt:    summary.SummarizedAt,
			}, true, nil
		}
	}
	return recordings.RecordingSummary{}, false, nil
}

func (s *transcriptionSummaryStoreSpy) UpsertMindMap(input recordings.UpsertMindMapInput) (recordings.RecordingMindMap, error) {
	s.mindMaps = append(s.mindMaps, input)
	return recordings.RecordingMindMap{
		RecordingID:     input.RecordingID,
		Provider:        input.Provider,
		Model:           input.Model,
		Title:           input.Title,
		RootJSON:        append([]byte(nil), input.RootJSON...),
		ContentMarkdown: input.ContentMarkdown,
		RawResultJSON:   append([]byte(nil), input.RawResultJSON...),
		GeneratedAt:     input.GeneratedAt,
	}, nil
}

type transcriptionProviderSpy struct {
	requests []TranscriptionRequest
	result   TranscriptionResult
	err      error
}

func (p *transcriptionProviderSpy) Transcribe(ctx context.Context, request TranscriptionRequest) (TranscriptionResult, error) {
	p.requests = append(p.requests, request)
	if p.err != nil {
		return TranscriptionResult{}, p.err
	}
	return p.result, nil
}

type summaryProviderSpy struct {
	requests []SummaryRequest
	result   SummaryResult
	err      error
}

func (p *summaryProviderSpy) Summarize(ctx context.Context, request SummaryRequest) (SummaryResult, error) {
	p.requests = append(p.requests, request)
	if p.err != nil {
		return SummaryResult{}, p.err
	}
	return p.result, nil
}

type mindMapProviderSpy struct {
	summaryProviderSpy
	mindMapRequests []MindMapRequest
	mindMapResult   MindMapResult
	mindMapErr      error
}

func (p *mindMapProviderSpy) GenerateMindMap(ctx context.Context, request MindMapRequest) (MindMapResult, error) {
	p.mindMapRequests = append(p.mindMapRequests, request)
	if p.mindMapErr != nil {
		return MindMapResult{}, p.mindMapErr
	}
	return p.mindMapResult, nil
}

type objectStoreSpy struct {
	deleted    []string
	objects    map[string]string
	urls       map[string]string
	err        error
	presignErr error
	gets       []string
	signs      []string
}

func (s *objectStoreSpy) PutObject(ctx context.Context, input storage.PutObjectInput) (storage.PutObjectResult, error) {
	return storage.PutObjectResult{Key: input.Key}, nil
}

func (s *objectStoreSpy) GetObject(ctx context.Context, key string) (storage.GetObjectResult, error) {
	s.gets = append(s.gets, key)
	return storage.GetObjectResult{Key: key, Body: io.NopCloser(strings.NewReader(s.objects[key])), SizeBytes: int64(len(s.objects[key]))}, nil
}

func (s *objectStoreSpy) PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error) {
	s.signs = append(s.signs, key)
	if s.presignErr != nil {
		return "", s.presignErr
	}
	if s.urls != nil {
		return s.urls[key], nil
	}
	return "", nil
}

func (s *objectStoreSpy) DeleteObject(ctx context.Context, key string) error {
	if s.err != nil {
		return s.err
	}
	s.deleted = append(s.deleted, key)
	return nil
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioUsesPresignedObjectURL(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_transcribe": {ID: "rec_transcribe", Language: "en", AudioObjectKey: "recordings/rec_transcribe/original.wav"},
		},
		normalizedAudio: recordings.RecordingNormalizedAudio{RecordingID: "rec_transcribe", ObjectKey: "recordings/rec_transcribe/normalized.wav"},
	}
	objectStore := &objectStoreSpy{urls: map[string]string{
		"recordings/rec_transcribe/normalized.wav": "https://objects.example.test/recordings/rec_transcribe/normalized.wav",
	}}
	provider := &transcriptionProviderSpy{result: TranscriptionResult{
		Provider:      "fake_transcription",
		Model:         "fake-whisper-v1",
		Language:      "en",
		Text:          "transcribed from object URL",
		TranscribedAt: time.Date(2026, 6, 6, 4, 5, 6, 0, time.UTC),
	}}
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, objectStore, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, provider, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err != nil {
		t.Fatalf("TranscribeRecordingAudio returned error: %v", err)
	}
	if len(objectStore.gets) != 0 {
		t.Fatalf("object store gets = %+v, want no normalized audio download", objectStore.gets)
	}
	if got, want := objectStore.signs, []string{"recordings/rec_transcribe/normalized.wav"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("object store signed keys = %+v, want %+v", got, want)
	}
	if provider.requests[0].AudioURL != "https://objects.example.test/recordings/rec_transcribe/normalized.wav" {
		t.Fatalf("provider AudioURL = %q, want presigned object URL", provider.requests[0].AudioURL)
	}
	if len(store.transcripts) != 1 || store.transcripts[0].Text != "transcribed from object URL" {
		t.Fatalf("stored transcripts = %+v, want provider transcript", store.transcripts)
	}
}

func newRecordingProcessingActivitiesForTest(store NormalizingPipelineStore, transcriptionProvider TranscriptionProvider, summaryProvider SummaryProvider) *RecordingProcessingActivities {
	return NewRecordingProcessingActivitiesWithNormalizedAudio(store, nil, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, transcriptionProvider, summaryProvider)
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioPersistsTranscript(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_transcribe": {
				ID:             "rec_transcribe",
				WorkflowType:   domain.WorkflowTypeMeeting,
				Language:       "en",
				AudioObjectKey: "recordings/rec_transcribe/original.wav",
			},
		},
		normalizedAudio: recordings.RecordingNormalizedAudio{RecordingID: "rec_transcribe", ObjectKey: "recordings/rec_transcribe/normalized.wav"},
	}
	objectStore := &objectStoreSpy{urls: map[string]string{
		"recordings/rec_transcribe/normalized.wav": "https://objects.example.test/recordings/rec_transcribe/normalized.wav",
	}}
	transcribedAt := time.Date(2026, 6, 6, 4, 5, 6, 0, time.UTC)
	provider := &transcriptionProviderSpy{result: TranscriptionResult{
		Provider:      "fake_transcription",
		Model:         "fake-whisper-v1",
		Language:      "en",
		Text:          "hello from rec_transcribe",
		RawResultJSON: []byte(`{"text":"hello from rec_transcribe"}`),
		TranscribedAt: transcribedAt,
		Segments: []TranscriptionSegmentResult{
			{SegmentIndex: 0, StartMS: 0, EndMS: 1200, SpeakerLabel: "speaker_1", Text: "hello from rec_transcribe", Confidence: 0.99},
		},
	}}
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, objectStore, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, provider, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err != nil {
		t.Fatalf("TranscribeRecordingAudio returned error: %v", err)
	}

	if len(provider.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(provider.requests))
	}
	request := provider.requests[0]
	if request.RecordingID != "rec_transcribe" || request.AudioURL != "https://objects.example.test/recordings/rec_transcribe/normalized.wav" || request.Language != "en" {
		t.Fatalf("provider request = %+v, want recording id/audio URL/language", request)
	}
	if len(store.transcripts) != 1 {
		t.Fatalf("stored transcripts = %d, want 1", len(store.transcripts))
	}
	transcript := store.transcripts[0]
	if transcript.RecordingID != "rec_transcribe" || transcript.Provider != "fake_transcription" || transcript.Text != "hello from rec_transcribe" {
		t.Fatalf("stored transcript = %+v, want provider transcript", transcript)
	}
	if len(transcript.Segments) != 1 || transcript.Segments[0].Text != "hello from rec_transcribe" {
		t.Fatalf("stored segments = %+v, want provider segment", transcript.Segments)
	}
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioPropagatesPresignError(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_transcribe": {
				ID:             "rec_transcribe",
				WorkflowType:   domain.WorkflowTypeMeeting,
				Language:       "en",
				AudioObjectKey: "recordings/rec_transcribe/original.wav",
			},
		},
		normalizedAudio: recordings.RecordingNormalizedAudio{RecordingID: "rec_transcribe", ObjectKey: "recordings/rec_transcribe/normalized.wav"},
	}
	presignErr := errors.New("presign failed")
	objectStore := &objectStoreSpy{presignErr: presignErr}
	provider := &transcriptionProviderSpy{}
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, objectStore, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, provider, &summaryProviderSpy{})

	err := activities.TranscribeRecordingAudio(context.Background(), "rec_transcribe")
	if !errors.Is(err, presignErr) {
		t.Fatalf("TranscribeRecordingAudio error = %v, want presign error", err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider requests = %+v, want no provider call", provider.requests)
	}
	if len(store.transcripts) != 0 {
		t.Fatalf("stored transcripts = %+v, want none", store.transcripts)
	}
}

func TestFakeTranscriptionProviderDoesNotPersistPresignedURLQuery(t *testing.T) {
	result, err := FakeTranscriptionProvider{}.Transcribe(context.Background(), TranscriptionRequest{
		RecordingID: "rec_fake",
		AudioURL:    "https://objects.example.test/workspaces/wsp/recordings/rec/normalized.wav?X-Amz-Signature=secret",
		Language:    "en",
	})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if !strings.Contains(result.Text, "normalized.wav") {
		t.Fatalf("text = %q, want normalized object label", result.Text)
	}
	if strings.Contains(result.Text, "X-Amz-Signature") || strings.Contains(result.Text, "secret") {
		t.Fatalf("text = %q, want no presigned URL query", result.Text)
	}
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioRejectsMissingRecordingID(t *testing.T) {
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(&transcriptionSummaryStoreSpy{}, &objectStoreSpy{}, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), ""); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want missing recording id error")
	}
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioRequiresDependencies(t *testing.T) {
	if err := NewRecordingProcessingActivitiesWithNormalizedAudio(nil, &objectStoreSpy{}, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{}).TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want store required error")
	}
	if err := NewRecordingProcessingActivitiesWithNormalizedAudio(&transcriptionSummaryStoreSpy{}, nil, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{}).TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want object store required error")
	}
	if err := NewRecordingProcessingActivitiesWithNormalizedAudio(&transcriptionSummaryStoreSpy{}, &objectStoreSpy{}, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, nil, &summaryProviderSpy{}).TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want transcription provider required error")
	}
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioRequiresNormalizedAudio(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{recordings: map[string]domain.Recording{
		"rec_transcribe": {ID: "rec_transcribe", Language: "en", AudioObjectKey: "recordings/rec_transcribe/original.wav"},
	}}
	provider := &transcriptionProviderSpy{}
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, &objectStoreSpy{}, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, provider, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want missing normalized audio error")
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider requests = %d, want 0 when normalized audio is missing", len(provider.requests))
	}
	if len(store.transcripts) != 0 {
		t.Fatalf("stored transcripts = %d, want 0 when normalized audio is missing", len(store.transcripts))
	}
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioPropagatesNormalizedAudioReadError(t *testing.T) {
	readErr := errors.New("normalized audio read failed")
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_transcribe": {ID: "rec_transcribe", Language: "en", AudioObjectKey: "recordings/rec_transcribe/original.wav"},
		},
		normalizedAudioErr: readErr,
	}
	provider := &transcriptionProviderSpy{}
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, &objectStoreSpy{}, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, provider, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), "rec_transcribe"); !errors.Is(err, readErr) {
		t.Fatalf("TranscribeRecordingAudio error = %v, want normalized audio read error", err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider requests = %d, want 0 when normalized audio read fails", len(provider.requests))
	}
	if len(store.transcripts) != 0 {
		t.Fatalf("stored transcripts = %d, want 0 when normalized audio read fails", len(store.transcripts))
	}
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioReturnsProviderErrorWithoutPersisting(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_transcribe": {ID: "rec_transcribe", Language: "en", AudioObjectKey: "recordings/rec_transcribe/original.wav"},
		},
		normalizedAudio: recordings.RecordingNormalizedAudio{RecordingID: "rec_transcribe", ObjectKey: "recordings/rec_transcribe/normalized.wav"},
	}
	providerErr := errors.New("transcription failed")
	objectStore := &objectStoreSpy{urls: map[string]string{
		"recordings/rec_transcribe/normalized.wav": "https://objects.example.test/recordings/rec_transcribe/normalized.wav",
	}}
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, objectStore, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, &transcriptionProviderSpy{err: providerErr}, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), "rec_transcribe"); !errors.Is(err, providerErr) {
		t.Fatalf("TranscribeRecordingAudio error = %v, want provider error", err)
	}
	if len(store.transcripts) != 0 {
		t.Fatalf("stored transcripts = %d, want 0 after provider error", len(store.transcripts))
	}
}

func TestRecordingProcessingActivitiesDeleteOriginalRecordingAudioRemovesOriginalObject(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{recordings: map[string]domain.Recording{
		"rec_delete_original": {ID: "rec_delete_original", AudioObjectKey: "recordings/rec_delete_original/original.wav"},
	}}
	objectStore := &objectStoreSpy{}
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, objectStore, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{})

	if err := activities.DeleteOriginalRecordingAudio(context.Background(), "rec_delete_original"); err != nil {
		t.Fatalf("DeleteOriginalRecordingAudio returned error: %v", err)
	}
	if got, want := objectStore.deleted, []string{"recordings/rec_delete_original/original.wav"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("deleted keys = %#v, want %#v", got, want)
	}
}

func TestRecordingProcessingActivitiesSummarizeRecordingPersistsSummary(t *testing.T) {
	summarizedAt := time.Date(2026, 6, 6, 5, 6, 7, 0, time.UTC)
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_summary": {ID: "rec_summary", Title: "Weekly sync", WorkflowType: domain.WorkflowTypeMeeting, Language: "en"},
		},
		transcript: recordings.RecordingTranscript{RecordingID: "rec_summary", Language: "en", Text: "hello world transcript"},
	}
	provider := &summaryProviderSpy{result: SummaryResult{
		Provider:        "fake_llm",
		Model:           "fake-summary-v1",
		Title:           "Weekly sync",
		Overview:        "hello world overview",
		ContentMarkdown: "# Weekly sync\n\nhello world overview",
		RawResultJSON:   []byte(`{"overview":"hello world overview"}`),
		SummarizedAt:    summarizedAt,
	}}
	activities := newRecordingProcessingActivitiesForTest(store, &transcriptionProviderSpy{}, provider)

	if err := activities.SummarizeRecording(context.Background(), "rec_summary"); err != nil {
		t.Fatalf("SummarizeRecording returned error: %v", err)
	}

	if len(provider.requests) != 1 {
		t.Fatalf("summary provider requests = %d, want 1", len(provider.requests))
	}
	request := provider.requests[0]
	if request.RecordingID != "rec_summary" || request.WorkflowType != domain.WorkflowTypeMeeting || request.TranscriptText != "hello world transcript" {
		t.Fatalf("summary request = %+v, want recording metadata and transcript text", request)
	}
	if len(store.summaries) != 1 {
		t.Fatalf("stored summaries = %d, want 1", len(store.summaries))
	}
	summary := store.summaries[0]
	if summary.RecordingID != "rec_summary" || summary.Provider != "fake_llm" || summary.Overview != "hello world overview" {
		t.Fatalf("stored summary = %+v, want provider summary", summary)
	}
}

func TestRecordingProcessingActivitiesSummarizeRecordingRejectsMissingRecordingID(t *testing.T) {
	activities := newRecordingProcessingActivitiesForTest(&transcriptionSummaryStoreSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{})

	if err := activities.SummarizeRecording(context.Background(), ""); err == nil {
		t.Fatal("SummarizeRecording returned nil error, want missing recording id error")
	}
}

func TestRecordingProcessingActivitiesSummarizeRecordingRequiresDependencies(t *testing.T) {
	if err := newRecordingProcessingActivitiesForTest(nil, &transcriptionProviderSpy{}, &summaryProviderSpy{}).SummarizeRecording(context.Background(), "rec_summary"); err == nil {
		t.Fatal("SummarizeRecording returned nil error, want store required error")
	}
	if err := newRecordingProcessingActivitiesForTest(&transcriptionSummaryStoreSpy{}, &transcriptionProviderSpy{}, nil).SummarizeRecording(context.Background(), "rec_summary"); err == nil {
		t.Fatal("SummarizeRecording returned nil error, want summary provider required error")
	}
}

func TestRecordingProcessingActivitiesSummarizeRecordingReturnsProviderErrorWithoutPersisting(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_summary": {ID: "rec_summary", Title: "Weekly sync", WorkflowType: domain.WorkflowTypeMeeting},
		},
		transcript: recordings.RecordingTranscript{RecordingID: "rec_summary", Text: "hello world transcript"},
	}
	providerErr := errors.New("summary failed")
	activities := newRecordingProcessingActivitiesForTest(store, &transcriptionProviderSpy{}, &summaryProviderSpy{err: providerErr})

	if err := activities.SummarizeRecording(context.Background(), "rec_summary"); !errors.Is(err, providerErr) {
		t.Fatalf("SummarizeRecording error = %v, want provider error", err)
	}
	if len(store.summaries) != 0 {
		t.Fatalf("stored summaries = %d, want 0 after provider error", len(store.summaries))
	}
}

func TestRecordingProcessingActivitiesGenerateMindMapPersistsMindMap(t *testing.T) {
	generatedAt := time.Date(2026, 6, 6, 6, 7, 8, 0, time.UTC)
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_mind_map": {ID: "rec_mind_map", Title: "Weekly sync", WorkflowType: domain.WorkflowTypeMeeting, Language: "en"},
		},
		transcript: recordings.RecordingTranscript{RecordingID: "rec_mind_map", Language: "en", Text: "launch status and dashboard action items"},
		summaries: []recordings.UpsertSummaryInput{{
			RecordingID:     "rec_mind_map",
			Provider:        "fake_llm",
			Model:           "fake-summary-v1",
			Type:            domain.WorkflowTypeMeeting,
			Title:           "Weekly sync",
			Overview:        "launch status",
			ContentMarkdown: "# Weekly sync\n\nLaunch status and dashboard action items.",
			RawResultJSON:   []byte(`{"overview":"launch status"}`),
			SummarizedAt:    generatedAt.Add(-time.Minute),
		}},
	}
	provider := &mindMapProviderSpy{mindMapResult: MindMapResult{
		Provider: "fake_llm",
		Model:    "fake-mind-map-v1",
		Title:    "Weekly sync",
		Root: MindMapNode{
			Label: "Weekly sync",
			Children: []MindMapNode{{
				Label:    "Launch status",
				Children: []MindMapNode{{Label: "Dashboard action items"}},
			}},
		},
		ContentMarkdown: "- Weekly sync\n  - Launch status\n    - Dashboard action items",
		RawResultJSON:   []byte(`{"title":"Weekly sync"}`),
		GeneratedAt:     generatedAt,
	}}
	activities := newRecordingProcessingActivitiesForTest(store, &transcriptionProviderSpy{}, provider)

	if err := activities.GenerateMindMap(context.Background(), "rec_mind_map"); err != nil {
		t.Fatalf("GenerateMindMap returned error: %v", err)
	}

	if len(provider.mindMapRequests) != 1 {
		t.Fatalf("mind map provider requests = %d, want 1", len(provider.mindMapRequests))
	}
	request := provider.mindMapRequests[0]
	if request.RecordingID != "rec_mind_map" || request.TranscriptText != "launch status and dashboard action items" || !strings.Contains(request.SummaryMarkdown, "Launch status") {
		t.Fatalf("mind map request = %+v, want recording metadata, transcript, and summary", request)
	}
	if len(store.mindMaps) != 1 {
		t.Fatalf("stored mind maps = %d, want 1", len(store.mindMaps))
	}
	mindMap := store.mindMaps[0]
	if mindMap.RecordingID != "rec_mind_map" || mindMap.Provider != "fake_llm" || !strings.Contains(string(mindMap.RootJSON), "Dashboard action items") {
		t.Fatalf("stored mind map = %+v, want provider mind map", mindMap)
	}
}

func TestRecordingProcessingActivitiesGenerateMindMapRequiresDependencies(t *testing.T) {
	if err := newRecordingProcessingActivitiesForTest(&transcriptionSummaryStoreSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{}).GenerateMindMap(context.Background(), "rec_mind_map"); err == nil {
		t.Fatal("GenerateMindMap returned nil error, want mind map provider required error")
	}
}
