package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
)

type transcriptionSummaryStoreSpy struct {
	recordings      map[string]domain.Recording
	normalizedAudio recordings.RecordingNormalizedAudio
	transcript      recordings.RecordingTranscript
	transcripts     []recordings.UpsertTranscriptInput
	summaries       []recordings.UpsertSummaryInput
}

func (s *transcriptionSummaryStoreSpy) Get(id string) (domain.Recording, bool) {
	if s.recordings == nil {
		return domain.Recording{}, false
	}
	recording, ok := s.recordings[id]
	return recording, ok
}

func (s *transcriptionSummaryStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	recording, ok := s.Get(input.ID)
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

func (s *transcriptionSummaryStoreSpy) GetNormalizedAudio(recordingID string) (recordings.RecordingNormalizedAudio, bool) {
	if s.normalizedAudio.RecordingID == "" || s.normalizedAudio.RecordingID != recordingID {
		return recordings.RecordingNormalizedAudio{}, false
	}
	return s.normalizedAudio, true
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

func (s *transcriptionSummaryStoreSpy) GetTranscript(recordingID string) (recordings.RecordingTranscript, bool) {
	if s.transcript.RecordingID == "" || s.transcript.RecordingID != recordingID {
		return recordings.RecordingTranscript{}, false
	}
	return s.transcript, true
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
	resolver := &localPathResolverSpy{path: "/tmp/soniq/recordings/rec_transcribe/normalized.wav"}
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
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, resolver, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, provider, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err != nil {
		t.Fatalf("TranscribeRecordingAudio returned error: %v", err)
	}

	if len(resolver.paths) != 1 || resolver.paths[0] != "recordings/rec_transcribe/normalized.wav" {
		t.Fatalf("resolved paths = %+v, want normalized object key", resolver.paths)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(provider.requests))
	}
	request := provider.requests[0]
	if request.RecordingID != "rec_transcribe" || request.AudioPath != "/tmp/soniq/recordings/rec_transcribe/normalized.wav" || request.Language != "en" {
		t.Fatalf("provider request = %+v, want recording id/local path/language", request)
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

func TestRecordingProcessingActivitiesTranscribeRecordingAudioRejectsMissingRecordingID(t *testing.T) {
	activities := NewRecordingProcessingActivitiesWithPipeline(&transcriptionSummaryStoreSpy{}, &localPathResolverSpy{}, &audioProbeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), ""); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want missing recording id error")
	}
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioRequiresDependencies(t *testing.T) {
	if err := NewRecordingProcessingActivitiesWithPipeline(nil, &localPathResolverSpy{}, &audioProbeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{}).TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want store required error")
	}
	if err := NewRecordingProcessingActivitiesWithPipeline(&transcriptionSummaryStoreSpy{}, nil, &audioProbeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{}).TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want path resolver required error")
	}
	if err := NewRecordingProcessingActivitiesWithPipeline(&transcriptionSummaryStoreSpy{}, &localPathResolverSpy{}, &audioProbeRunnerSpy{}, nil, &summaryProviderSpy{}).TranscribeRecordingAudio(context.Background(), "rec_transcribe"); err == nil {
		t.Fatal("TranscribeRecordingAudio returned nil error, want transcription provider required error")
	}
}

func TestRecordingProcessingActivitiesTranscribeRecordingAudioRequiresNormalizedAudio(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{recordings: map[string]domain.Recording{
		"rec_transcribe": {ID: "rec_transcribe", Language: "en", AudioObjectKey: "recordings/rec_transcribe/original.wav"},
	}}
	provider := &transcriptionProviderSpy{}
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, &localPathResolverSpy{path: "/tmp/soniq/recordings/rec_transcribe/normalized.wav"}, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, provider, &summaryProviderSpy{})

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

func TestRecordingProcessingActivitiesTranscribeRecordingAudioReturnsProviderErrorWithoutPersisting(t *testing.T) {
	store := &transcriptionSummaryStoreSpy{
		recordings: map[string]domain.Recording{
			"rec_transcribe": {ID: "rec_transcribe", Language: "en", AudioObjectKey: "recordings/rec_transcribe/original.wav"},
		},
		normalizedAudio: recordings.RecordingNormalizedAudio{RecordingID: "rec_transcribe", ObjectKey: "recordings/rec_transcribe/normalized.wav"},
	}
	providerErr := errors.New("transcription failed")
	activities := NewRecordingProcessingActivitiesWithNormalizedAudio(store, &localPathResolverSpy{path: "/tmp/audio.wav"}, &audioProbeRunnerSpy{}, &audioNormalizeRunnerSpy{}, &transcriptionProviderSpy{err: providerErr}, &summaryProviderSpy{})

	if err := activities.TranscribeRecordingAudio(context.Background(), "rec_transcribe"); !errors.Is(err, providerErr) {
		t.Fatalf("TranscribeRecordingAudio error = %v, want provider error", err)
	}
	if len(store.transcripts) != 0 {
		t.Fatalf("stored transcripts = %d, want 0 after provider error", len(store.transcripts))
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
	activities := NewRecordingProcessingActivitiesWithPipeline(store, &localPathResolverSpy{}, &audioProbeRunnerSpy{}, &transcriptionProviderSpy{}, provider)

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
	activities := NewRecordingProcessingActivitiesWithPipeline(&transcriptionSummaryStoreSpy{}, &localPathResolverSpy{}, &audioProbeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{})

	if err := activities.SummarizeRecording(context.Background(), ""); err == nil {
		t.Fatal("SummarizeRecording returned nil error, want missing recording id error")
	}
}

func TestRecordingProcessingActivitiesSummarizeRecordingRequiresDependencies(t *testing.T) {
	if err := NewRecordingProcessingActivitiesWithPipeline(nil, &localPathResolverSpy{}, &audioProbeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{}).SummarizeRecording(context.Background(), "rec_summary"); err == nil {
		t.Fatal("SummarizeRecording returned nil error, want store required error")
	}
	if err := NewRecordingProcessingActivitiesWithPipeline(&transcriptionSummaryStoreSpy{}, &localPathResolverSpy{}, &audioProbeRunnerSpy{}, &transcriptionProviderSpy{}, nil).SummarizeRecording(context.Background(), "rec_summary"); err == nil {
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
	activities := NewRecordingProcessingActivitiesWithPipeline(store, &localPathResolverSpy{}, &audioProbeRunnerSpy{}, &transcriptionProviderSpy{}, &summaryProviderSpy{err: providerErr})

	if err := activities.SummarizeRecording(context.Background(), "rec_summary"); !errors.Is(err, providerErr) {
		t.Fatalf("SummarizeRecording error = %v, want provider error", err)
	}
	if len(store.summaries) != 0 {
		t.Fatalf("stored summaries = %d, want 0 after provider error", len(store.summaries))
	}
}
