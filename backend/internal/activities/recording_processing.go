package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

// RecordingProcessingInput is the input shared by the recording processing workflow and activities.
type RecordingProcessingInput struct {
	WorkspaceID                           string
	RecordingID                           string
	WorkflowType                          domain.WorkflowType
	Language                              string
	DeleteOriginalAudioAfterTranscription bool
}

// RecordingReference identifies a recording inside a workspace.
type RecordingReference struct {
	WorkspaceID string
	RecordingID string
}

// RecordingFailure contains failure metadata to persist for a recording.
type RecordingFailure struct {
	WorkspaceID string
	RecordingID string
	Reason      string
}

// RecordingProcessingResult is the result returned after processing completes.
type RecordingProcessingResult struct {
	WorkspaceID string
	RecordingID string
	Status      domain.RecordingStatus
}

const (
	ValidateRecordingActivityName            = "ValidateRecordingActivity"
	MarkRecordingProcessingActivityName      = "MarkRecordingProcessingActivity"
	ProbeRecordingAudioActivityName          = "ProbeRecordingAudioActivity"
	NormalizeRecordingAudioActivityName      = "NormalizeRecordingAudioActivity"
	MarkRecordingTranscribingActivityName    = "MarkRecordingTranscribingActivity"
	TranscribeRecordingAudioActivityName     = "TranscribeRecordingAudioActivity"
	MarkRecordingSummarizingActivityName     = "MarkRecordingSummarizingActivity"
	SummarizeRecordingActivityName           = "SummarizeRecordingActivity"
	GenerateMindMapActivityName              = "GenerateMindMapActivity"
	DeleteOriginalRecordingAudioActivityName = "DeleteOriginalRecordingAudioActivity"
	CompleteRecordingProcessingActivityName  = "CompleteRecordingProcessingActivity"
	FailRecordingProcessingActivityName      = "FailRecordingProcessingActivity"
)

// RecordingStore is the persistence seam used by validation, status, and audio probe activities.
type RecordingStore interface {
	Get(id string) (domain.Recording, bool, error)
	GetForWorkspace(input recordings.GetRecordingInput) (domain.Recording, bool, error)
	UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error)
	UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error)
}

// NormalizedAudioStore is the normalized audio persistence seam used by normalization activities.
type NormalizedAudioStore interface {
	UpsertNormalizedAudio(input recordings.UpsertNormalizedAudioInput) (recordings.RecordingNormalizedAudio, error)
	GetNormalizedAudio(recordingID string) (recordings.RecordingNormalizedAudio, bool, error)
}

// TranscriptStore is the transcript persistence seam used by transcription and summarization activities.
type TranscriptStore interface {
	UpsertTranscript(input recordings.UpsertTranscriptInput) (recordings.RecordingTranscript, error)
	GetTranscript(recordingID string) (recordings.RecordingTranscript, bool, error)
}

// SummaryStore is the summary persistence seam used by summarization activities.
type SummaryStore interface {
	UpsertSummary(input recordings.UpsertSummaryInput) (recordings.RecordingSummary, error)
	GetSummary(recordingID string) (recordings.RecordingSummary, bool, error)
}

// MindMapStore is the mind map persistence seam used by mind map activities.
type MindMapStore interface {
	UpsertMindMap(input recordings.UpsertMindMapInput) (recordings.RecordingMindMap, error)
}

// PipelineStore is the complete persistence seam used by the transcription/summarization pipeline.
type PipelineStore interface {
	RecordingStore
	TranscriptStore
	SummaryStore
}

// NormalizingPipelineStore is the complete persistence seam used once normalization participates in the activity set.
type NormalizingPipelineStore interface {
	PipelineStore
	NormalizedAudioStore
}

// LocalObjectPathResolver resolves stored object keys to local filesystem paths.
type LocalObjectPathResolver interface {
	LocalPathForObject(key string) (string, error)
}

// AudioProbeRunner probes an audio file and returns normalized metadata.
type AudioProbeRunner interface {
	Probe(ctx context.Context, path string) (AudioProbeResult, error)
}

// AudioProbeResult contains normalized ffprobe metadata for persistence.
type AudioProbeResult struct {
	DurationSeconds float64
	FormatName      string
	CodecName       string
	SampleRate      int
	Channels        int
	BitRate         int
	RawProbeJSON    []byte
	ProbedAt        time.Time
}

// TranscriptionProvider converts a local audio file into transcript text and segments.
type TranscriptionProvider interface {
	Transcribe(ctx context.Context, request TranscriptionRequest) (TranscriptionResult, error)
}

// TranscriptionRequest contains local audio metadata for transcription.
type TranscriptionRequest struct {
	RecordingID string
	AudioPath   string
	Language    string
}

// TranscriptionResult contains provider-neutral transcription output.
type TranscriptionResult struct {
	Provider      string
	Model         string
	Language      string
	Text          string
	RawResultJSON []byte
	TranscribedAt time.Time
	Segments      []TranscriptionSegmentResult
}

// TranscriptionSegmentResult contains one provider-neutral transcript segment.
type TranscriptionSegmentResult struct {
	SegmentIndex int
	StartMS      int
	EndMS        int
	SpeakerLabel string
	Text         string
	Confidence   float64
}

// SummaryProvider converts transcript text and recording metadata into a summary.
type SummaryProvider interface {
	Summarize(ctx context.Context, request SummaryRequest) (SummaryResult, error)
}

// SummaryRequest contains provider-neutral summarization input.
type SummaryRequest struct {
	RecordingID    string
	Title          string
	WorkflowType   domain.WorkflowType
	Language       string
	TranscriptText string
}

// SummaryResult contains provider-neutral summary output.
type SummaryResult struct {
	Provider        string
	Model           string
	Title           string
	Overview        string
	ContentMarkdown string
	RawResultJSON   []byte
	SummarizedAt    time.Time
}

// MindMapProvider converts transcript/summary text into a structured mind map.
type MindMapProvider interface {
	GenerateMindMap(ctx context.Context, request MindMapRequest) (MindMapResult, error)
}

// MindMapRequest contains provider-neutral mind map input.
type MindMapRequest struct {
	RecordingID     string
	Title           string
	WorkflowType    domain.WorkflowType
	Language        string
	TranscriptText  string
	SummaryMarkdown string
}

// MindMapNode is the provider-neutral tree node exposed by generated mind maps.
type MindMapNode struct {
	Label    string        `json:"label"`
	Children []MindMapNode `json:"children,omitempty"`
}

// MindMapResult contains provider-neutral mind map output.
type MindMapResult struct {
	Provider        string
	Model           string
	Title           string
	Root            MindMapNode
	ContentMarkdown string
	RawResultJSON   []byte
	GeneratedAt     time.Time
}

// FFProbeRunner runs the ffprobe binary.
type FFProbeRunner struct {
	Binary string
}

// RecordingProcessingActivities contains store-backed Temporal activity methods.
type RecordingProcessingActivities struct {
	store                 RecordingStore
	normalizedAudioStore  NormalizedAudioStore
	transcriptStore       TranscriptStore
	summaryStore          SummaryStore
	mindMapStore          MindMapStore
	objectStore           storage.ObjectStore
	pathResolver          LocalObjectPathResolver
	probeRunner           AudioProbeRunner
	normalizeRunner       AudioNormalizeRunner
	transcriptionProvider TranscriptionProvider
	summaryProvider       SummaryProvider
	mindMapProvider       MindMapProvider
}

// NewRecordingProcessingActivities creates store-backed recording processing activities.
func NewRecordingProcessingActivities(store RecordingStore) *RecordingProcessingActivities {
	return &RecordingProcessingActivities{store: store}
}

// NewRecordingProcessingActivitiesWithAudioProbe creates recording processing activities with audio probe dependencies.
func NewRecordingProcessingActivitiesWithAudioProbe(store RecordingStore, resolver LocalObjectPathResolver, runner AudioProbeRunner) *RecordingProcessingActivities {
	return &RecordingProcessingActivities{store: store, pathResolver: resolver, probeRunner: runner}
}

// NewRecordingProcessingActivitiesWithPipeline creates recording processing activities with all local pipeline dependencies.
func NewRecordingProcessingActivitiesWithPipeline(store PipelineStore, resolver LocalObjectPathResolver, runner AudioProbeRunner, transcriptionProvider TranscriptionProvider, summaryProvider SummaryProvider) *RecordingProcessingActivities {
	mindMapStore, _ := any(store).(MindMapStore)
	mindMapProvider, _ := summaryProvider.(MindMapProvider)
	return &RecordingProcessingActivities{
		store:                 store,
		transcriptStore:       store,
		summaryStore:          store,
		mindMapStore:          mindMapStore,
		pathResolver:          resolver,
		probeRunner:           runner,
		transcriptionProvider: transcriptionProvider,
		summaryProvider:       summaryProvider,
		mindMapProvider:       mindMapProvider,
	}
}

// NewRecordingProcessingActivitiesWithNormalizedAudio creates recording processing activities with normalization dependencies.
func NewRecordingProcessingActivitiesWithNormalizedAudio(store NormalizingPipelineStore, resolver LocalObjectPathResolver, objectStore storage.ObjectStore, probeRunner AudioProbeRunner, normalizeRunner AudioNormalizeRunner, transcriptionProvider TranscriptionProvider, summaryProvider SummaryProvider) *RecordingProcessingActivities {
	activities := NewRecordingProcessingActivitiesWithPipeline(store, resolver, probeRunner, transcriptionProvider, summaryProvider)
	activities.normalizedAudioStore = store
	activities.objectStore = objectStore
	activities.normalizeRunner = normalizeRunner
	return activities
}

// ValidateRecording validates processing input and confirms the recording exists.
func (a *RecordingProcessingActivities) ValidateRecording(ctx context.Context, input RecordingProcessingInput) error {
	if input.WorkspaceID == "" {
		return errors.New("workspace id is required")
	}
	if input.RecordingID == "" {
		return errors.New("recording id is required")
	}
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return errors.New("workflow type is invalid")
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	_, ok, err := a.store.GetForWorkspace(recordings.GetRecordingInput{
		WorkspaceID: input.WorkspaceID,
		ID:          input.RecordingID,
	})
	if err != nil {
		return fmt.Errorf("get recording: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording not found: %s", input.RecordingID)
	}
	return nil
}

// MarkRecordingProcessing persists the processing status transition.
func (a *RecordingProcessingActivities) MarkRecordingProcessing(ctx context.Context, recording RecordingReference) error {
	_, err := a.updateStatus(recording, domain.RecordingStatusProcessing)
	return err
}

// MarkRecordingTranscribing persists the transcribing status transition.
func (a *RecordingProcessingActivities) MarkRecordingTranscribing(ctx context.Context, recording RecordingReference) error {
	_, err := a.updateStatus(recording, domain.RecordingStatusTranscribing)
	return err
}

// MarkRecordingSummarizing persists the summarizing status transition.
func (a *RecordingProcessingActivities) MarkRecordingSummarizing(ctx context.Context, recording RecordingReference) error {
	_, err := a.updateStatus(recording, domain.RecordingStatusSummarizing)
	return err
}

// CompleteRecordingProcessing persists completion and returns the workflow result.
func (a *RecordingProcessingActivities) CompleteRecordingProcessing(ctx context.Context, recording RecordingReference) (RecordingProcessingResult, error) {
	updated, err := a.updateStatus(recording, domain.RecordingStatusCompleted)
	if err != nil {
		return RecordingProcessingResult{}, err
	}
	return RecordingProcessingResult{
		WorkspaceID: updated.WorkspaceID,
		RecordingID: updated.ID,
		Status:      updated.Status,
	}, nil
}

// FailRecordingProcessing persists a failed status transition.
func (a *RecordingProcessingActivities) FailRecordingProcessing(ctx context.Context, failure RecordingFailure) error {
	_, err := a.updateStatus(RecordingReference{
		WorkspaceID: failure.WorkspaceID,
		RecordingID: failure.RecordingID,
	}, domain.RecordingStatusFailed, failure.Reason)
	return err
}

// ProbeRecordingAudio probes the original uploaded audio and persists ffprobe metadata.
func (a *RecordingProcessingActivities) ProbeRecordingAudio(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if a.pathResolver == nil {
		return errors.New("audio object path resolver is required")
	}
	if a.probeRunner == nil {
		return errors.New("audio probe runner is required")
	}

	recording, ok, err := a.store.Get(recordingID)
	if err != nil {
		return fmt.Errorf("get recording: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording not found: %s", recordingID)
	}
	if strings.TrimSpace(recording.AudioObjectKey) == "" {
		return fmt.Errorf("recording audio object key is required: %s", recordingID)
	}

	path, err := a.pathResolver.LocalPathForObject(recording.AudioObjectKey)
	if err != nil {
		return fmt.Errorf("resolve recording audio object path: %w", err)
	}
	probe, err := a.probeRunner.Probe(ctx, path)
	if err != nil {
		return fmt.Errorf("probe recording audio: %w", err)
	}
	_, err = a.store.UpsertAudioProbe(recordings.UpsertAudioProbeInput{
		RecordingID:     recordingID,
		DurationSeconds: probe.DurationSeconds,
		FormatName:      probe.FormatName,
		CodecName:       probe.CodecName,
		SampleRate:      probe.SampleRate,
		Channels:        probe.Channels,
		BitRate:         probe.BitRate,
		RawProbeJSON:    append([]byte(nil), probe.RawProbeJSON...),
		ProbedAt:        probe.ProbedAt,
	})
	if err != nil {
		return fmt.Errorf("persist recording audio probe: %w", err)
	}
	return nil
}

// NormalizeRecordingAudio normalizes the original recording audio and persists normalized artifact metadata.
func (a *RecordingProcessingActivities) NormalizeRecordingAudio(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if a.normalizedAudioStore == nil {
		return errors.New("normalized audio store is required")
	}
	if a.pathResolver == nil {
		return errors.New("audio object path resolver is required")
	}
	if a.normalizeRunner == nil {
		return errors.New("audio normalize runner is required")
	}

	recording, ok, err := a.store.Get(recordingID)
	if err != nil {
		return fmt.Errorf("get recording: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording not found: %s", recordingID)
	}
	if strings.TrimSpace(recording.AudioObjectKey) == "" {
		return fmt.Errorf("recording audio object key is required: %s", recordingID)
	}
	inputPath, err := a.pathResolver.LocalPathForObject(recording.AudioObjectKey)
	if err != nil {
		return fmt.Errorf("resolve recording audio object path: %w", err)
	}
	normalizedObjectKey, err := storage.NormalizedAudioObjectKey(recording.AudioObjectKey)
	if err != nil {
		return fmt.Errorf("build normalized audio object key: %w", err)
	}
	outputPath, err := a.pathResolver.LocalPathForObject(normalizedObjectKey)
	if err != nil {
		return fmt.Errorf("resolve normalized audio object path: %w", err)
	}
	result, err := a.normalizeRunner.Normalize(ctx, AudioNormalizeRequest{InputPath: inputPath, OutputPath: outputPath})
	if err != nil {
		return fmt.Errorf("normalize recording audio: %w", err)
	}
	stat, err := os.Stat(result.OutputPath)
	if err != nil {
		return fmt.Errorf("stat normalized audio: %w", err)
	}
	_, err = a.normalizedAudioStore.UpsertNormalizedAudio(recordings.UpsertNormalizedAudioInput{
		RecordingID:  recordingID,
		ObjectKey:    normalizedObjectKey,
		ContentType:  result.ContentType,
		SizeBytes:    stat.Size(),
		FormatName:   result.FormatName,
		CodecName:    result.CodecName,
		SampleRate:   result.SampleRate,
		Channels:     result.Channels,
		NormalizedAt: result.NormalizedAt,
	})
	if err != nil {
		return fmt.Errorf("upsert normalized audio: %w", err)
	}
	return nil
}

// TranscribeRecordingAudio transcribes normalized recording audio and persists transcript output.
func (a *RecordingProcessingActivities) TranscribeRecordingAudio(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if a.normalizedAudioStore == nil {
		return errors.New("normalized audio store is required")
	}
	if a.transcriptStore == nil {
		return errors.New("transcript store is required")
	}
	if a.pathResolver == nil {
		return errors.New("audio object path resolver is required")
	}
	if a.transcriptionProvider == nil {
		return errors.New("transcription provider is required")
	}
	recording, ok, err := a.store.Get(recordingID)
	if err != nil {
		return fmt.Errorf("get recording: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording not found: %s", recordingID)
	}
	normalizedAudio, ok, err := a.normalizedAudioStore.GetNormalizedAudio(recordingID)
	if err != nil {
		return fmt.Errorf("get recording normalized audio: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording normalized audio not found: %s", recordingID)
	}
	if strings.TrimSpace(normalizedAudio.ObjectKey) == "" {
		return fmt.Errorf("recording normalized audio object key is required: %s", recordingID)
	}
	path, err := a.pathResolver.LocalPathForObject(normalizedAudio.ObjectKey)
	if err != nil {
		return fmt.Errorf("resolve normalized audio object path: %w", err)
	}
	result, err := a.transcriptionProvider.Transcribe(ctx, TranscriptionRequest{
		RecordingID: recordingID,
		AudioPath:   path,
		Language:    recording.Language,
	})
	if err != nil {
		return fmt.Errorf("transcribe recording audio: %w", err)
	}
	segments := make([]recordings.UpsertTranscriptSegmentInput, 0, len(result.Segments))
	for _, segment := range result.Segments {
		segments = append(segments, recordings.UpsertTranscriptSegmentInput{
			SegmentIndex: segment.SegmentIndex,
			StartMS:      segment.StartMS,
			EndMS:        segment.EndMS,
			SpeakerLabel: segment.SpeakerLabel,
			Text:         segment.Text,
			Confidence:   segment.Confidence,
		})
	}
	_, err = a.transcriptStore.UpsertTranscript(recordings.UpsertTranscriptInput{
		RecordingID:   recordingID,
		Provider:      result.Provider,
		Model:         result.Model,
		Language:      result.Language,
		Text:          result.Text,
		RawResultJSON: append([]byte(nil), result.RawResultJSON...),
		TranscribedAt: result.TranscribedAt,
		Segments:      segments,
	})
	if err != nil {
		return fmt.Errorf("persist recording transcript: %w", err)
	}
	return nil
}

// DeleteOriginalRecordingAudio removes the original uploaded audio object after transcription.
func (a *RecordingProcessingActivities) DeleteOriginalRecordingAudio(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if a.objectStore == nil {
		return errors.New("object store is required")
	}
	recording, ok, err := a.store.Get(recordingID)
	if err != nil {
		return fmt.Errorf("get recording: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording not found: %s", recordingID)
	}
	if strings.TrimSpace(recording.AudioObjectKey) == "" {
		return errors.New("recording audio object key is required")
	}
	if err := a.objectStore.DeleteObject(ctx, recording.AudioObjectKey); err != nil {
		return fmt.Errorf("delete original recording audio: %w", err)
	}
	return nil
}

// SummarizeRecording summarizes the latest transcript and persists summary output.
func (a *RecordingProcessingActivities) SummarizeRecording(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if a.transcriptStore == nil {
		return errors.New("transcript store is required")
	}
	if a.summaryStore == nil {
		return errors.New("summary store is required")
	}
	if a.summaryProvider == nil {
		return errors.New("summary provider is required")
	}
	recording, ok, err := a.store.Get(recordingID)
	if err != nil {
		return fmt.Errorf("get recording: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording not found: %s", recordingID)
	}
	transcript, ok, err := a.transcriptStore.GetTranscript(recordingID)
	if err != nil {
		return fmt.Errorf("get recording transcript: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording transcript not found: %s", recordingID)
	}
	result, err := a.summaryProvider.Summarize(ctx, SummaryRequest{
		RecordingID:    recordingID,
		Title:          recording.Title,
		WorkflowType:   recording.WorkflowType,
		Language:       recording.Language,
		TranscriptText: transcript.Text,
	})
	if err != nil {
		return fmt.Errorf("summarize recording: %w", err)
	}
	_, err = a.summaryStore.UpsertSummary(recordings.UpsertSummaryInput{
		RecordingID:     recordingID,
		Provider:        result.Provider,
		Model:           result.Model,
		Type:            recording.WorkflowType,
		Title:           result.Title,
		Overview:        result.Overview,
		ContentMarkdown: result.ContentMarkdown,
		RawResultJSON:   append([]byte(nil), result.RawResultJSON...),
		SummarizedAt:    result.SummarizedAt,
	})
	if err != nil {
		return fmt.Errorf("persist recording summary: %w", err)
	}
	return nil
}

// GenerateMindMap generates and persists a structured mind map from the transcript and summary.
func (a *RecordingProcessingActivities) GenerateMindMap(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if a.transcriptStore == nil {
		return errors.New("transcript store is required")
	}
	if a.summaryStore == nil {
		return errors.New("summary store is required")
	}
	if a.mindMapStore == nil {
		return errors.New("mind map store is required")
	}
	if a.mindMapProvider == nil {
		return errors.New("mind map provider is required")
	}
	recording, ok, err := a.store.Get(recordingID)
	if err != nil {
		return fmt.Errorf("get recording: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording not found: %s", recordingID)
	}
	transcript, ok, err := a.transcriptStore.GetTranscript(recordingID)
	if err != nil {
		return fmt.Errorf("get recording transcript: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording transcript not found: %s", recordingID)
	}
	summary, ok, err := a.summaryStore.GetSummary(recordingID)
	if err != nil {
		return fmt.Errorf("get recording summary: %w", err)
	}
	if !ok {
		return fmt.Errorf("recording summary not found: %s", recordingID)
	}
	result, err := a.mindMapProvider.GenerateMindMap(ctx, MindMapRequest{
		RecordingID:     recordingID,
		Title:           recording.Title,
		WorkflowType:    recording.WorkflowType,
		Language:        recording.Language,
		TranscriptText:  transcript.Text,
		SummaryMarkdown: summary.ContentMarkdown,
	})
	if err != nil {
		return fmt.Errorf("generate recording mind map: %w", err)
	}
	rootJSON, err := json.Marshal(result.Root)
	if err != nil {
		return fmt.Errorf("marshal recording mind map root: %w", err)
	}
	_, err = a.mindMapStore.UpsertMindMap(recordings.UpsertMindMapInput{
		RecordingID:     recordingID,
		Provider:        result.Provider,
		Model:           result.Model,
		Title:           result.Title,
		RootJSON:        rootJSON,
		ContentMarkdown: result.ContentMarkdown,
		RawResultJSON:   append([]byte(nil), result.RawResultJSON...),
		GeneratedAt:     result.GeneratedAt,
	})
	if err != nil {
		return fmt.Errorf("persist recording mind map: %w", err)
	}
	return nil
}

// FakeTranscriptionProvider is a deterministic local transcription provider for tests and smoke runs.
type FakeTranscriptionProvider struct{}

func (p FakeTranscriptionProvider) Transcribe(ctx context.Context, request TranscriptionRequest) (TranscriptionResult, error) {
	base := filepath.Base(request.AudioPath)
	text := fmt.Sprintf("Fake transcript for %s from %s", request.RecordingID, base)
	raw, _ := json.Marshal(map[string]string{"text": text})
	return TranscriptionResult{
		Provider:      "fake_transcription",
		Model:         "fake-transcription-v1",
		Language:      request.Language,
		Text:          text,
		RawResultJSON: raw,
		TranscribedAt: time.Now().UTC(),
		Segments: []TranscriptionSegmentResult{{
			SegmentIndex: 0,
			StartMS:      0,
			EndMS:        1000,
			SpeakerLabel: "speaker_1",
			Text:         text,
			Confidence:   1,
		}},
	}, nil
}

// FakeSummaryProvider is a deterministic local summarization provider for tests and smoke runs.
type FakeSummaryProvider struct{}

func (p FakeSummaryProvider) Summarize(ctx context.Context, request SummaryRequest) (SummaryResult, error) {
	overview := strings.TrimSpace(request.TranscriptText)
	overview = truncateRunes(overview, 120)
	if overview == "" {
		overview = "No transcript text available."
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = request.RecordingID
	}
	markdown := fmt.Sprintf("# %s\n\n%s", title, overview)
	raw, _ := json.Marshal(map[string]string{"overview": overview})
	return SummaryResult{
		Provider:        "fake_llm",
		Model:           "fake-summary-v1",
		Title:           title,
		Overview:        overview,
		ContentMarkdown: markdown,
		RawResultJSON:   raw,
		SummarizedAt:    time.Now().UTC(),
	}, nil
}

// GenerateMindMap creates a deterministic local mind map for tests and smoke runs.
func (p FakeSummaryProvider) GenerateMindMap(ctx context.Context, request MindMapRequest) (MindMapResult, error) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = request.RecordingID
	}
	overview := strings.TrimSpace(request.SummaryMarkdown)
	if overview == "" {
		overview = strings.TrimSpace(request.TranscriptText)
	}
	overview = truncateRunes(overview, 80)
	if overview == "" {
		overview = "No transcript text available."
	}
	root := MindMapNode{
		Label: title,
		Children: []MindMapNode{
			{Label: "Overview", Children: []MindMapNode{{Label: overview}}},
			{Label: "Workflow", Children: []MindMapNode{{Label: string(request.WorkflowType)}}},
		},
	}
	raw, _ := json.Marshal(map[string]any{"title": title, "root": root})
	return MindMapResult{
		Provider:        "fake_llm",
		Model:           "fake-mind-map-v1",
		Title:           title,
		Root:            root,
		ContentMarkdown: mindMapMarkdown(root),
		RawResultJSON:   raw,
		GeneratedAt:     time.Now().UTC(),
	}, nil
}

func (r FFProbeRunner) Probe(ctx context.Context, path string) (AudioProbeResult, error) {
	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "ffprobe"
	}
	stat, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AudioProbeResult{}, fmt.Errorf("audio file not found: %s", path)
		}
		return AudioProbeResult{}, fmt.Errorf("stat audio file: %w", err)
	}
	if stat.IsDir() {
		return AudioProbeResult{}, fmt.Errorf("audio path is a directory: %s", path)
	}

	output, err := exec.CommandContext(ctx, binary, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path).CombinedOutput()
	if err != nil {
		if detail := strings.TrimSpace(string(output)); detail != "" {
			return AudioProbeResult{}, fmt.Errorf("run ffprobe: %s: %w", detail, err)
		}
		return AudioProbeResult{}, fmt.Errorf("run ffprobe: %w", err)
	}
	return parseFFProbeOutput(output, time.Now().UTC())
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
	BitRate    string `json:"bit_rate"`
}

type ffprobeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
}

func parseFFProbeOutput(raw []byte, probedAt time.Time) (AudioProbeResult, error) {
	var parsed ffprobeOutput
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return AudioProbeResult{}, fmt.Errorf("parse ffprobe json: %w", err)
	}

	result := AudioProbeResult{
		FormatName:   parsed.Format.FormatName,
		BitRate:      parseIntOrZero(parsed.Format.BitRate),
		RawProbeJSON: append([]byte(nil), raw...),
		ProbedAt:     probedAt,
	}
	if parsed.Format.Duration != "" {
		duration, err := strconv.ParseFloat(parsed.Format.Duration, 64)
		if err != nil {
			return AudioProbeResult{}, fmt.Errorf("parse ffprobe duration: %w", err)
		}
		result.DurationSeconds = duration
	}

	for _, stream := range parsed.Streams {
		if stream.CodecType != "audio" {
			continue
		}
		result.CodecName = stream.CodecName
		result.SampleRate = parseIntOrZero(stream.SampleRate)
		result.Channels = stream.Channels
		if stream.BitRate != "" {
			result.BitRate = parseIntOrZero(stream.BitRate)
		}
		break
	}
	if strings.TrimSpace(result.FormatName) == "" {
		return AudioProbeResult{}, errors.New("ffprobe format_name is required")
	}
	return result, nil
}

func parseIntOrZero(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func (a *RecordingProcessingActivities) updateStatus(recording RecordingReference, status domain.RecordingStatus, failureReason ...string) (domain.Recording, error) {
	if recording.WorkspaceID == "" {
		return domain.Recording{}, errors.New("workspace id is required")
	}
	if recording.RecordingID == "" {
		return domain.Recording{}, errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return domain.Recording{}, errors.New("recording store is required")
	}
	reason := ""
	if len(failureReason) > 0 {
		reason = failureReason[0]
	}
	updated, err := a.store.UpdateStatus(recordings.UpdateRecordingStatusInput{
		WorkspaceID:   recording.WorkspaceID,
		ID:            recording.RecordingID,
		Status:        status,
		FailureReason: reason,
	})
	if err != nil {
		return domain.Recording{}, err
	}
	return updated, nil
}
