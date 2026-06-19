package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/activities"
	"github.com/zzyhdu/soniq/backend/internal/config"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/activity"
)

func TestInterruptChFromContextClosesWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	interruptCh := interruptChFromContext(ctx)

	cancel()

	select {
	case <-interruptCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interrupt channel to close")
	}
}

func TestRunTemporalWorkerWithCleanupStopsCleanupOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &temporalWorkerRunnerSpy{started: make(chan struct{})}
	cleanupRunner := &backgroundRunnerSpy{started: make(chan struct{}), stopped: make(chan struct{})}
	errCh := make(chan error, 1)
	go func() {
		errCh <- runTemporalWorkerWithCleanup(ctx, worker, cleanupRunner)
	}()

	select {
	case <-worker.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker to start")
	}
	select {
	case <-cleanupRunner.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cleanup runner to start")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runTemporalWorkerWithCleanup returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker shutdown")
	}
	select {
	case <-cleanupRunner.stopped:
	default:
		t.Fatal("cleanup runner stopped after function returned = false, want true")
	}
}

func TestRunTemporalWorkerWithCleanupReturnsWorkerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wantErr := errors.New("worker failed")
	worker := &temporalWorkerRunnerSpy{started: make(chan struct{}), err: wantErr}
	cleanupRunner := &backgroundRunnerSpy{started: make(chan struct{}), stopped: make(chan struct{})}

	err := runTemporalWorkerWithCleanup(ctx, worker, cleanupRunner)

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	select {
	case <-cleanupRunner.stopped:
	default:
		t.Fatal("cleanup runner stopped after worker error = false, want true")
	}
}

func TestRegisterRecordingProcessingRegistersWorkflowAndActivities(t *testing.T) {
	worker := &recordingWorkerSpy{}
	store := &workerRecordingStoreSpy{}

	registerRecordingProcessing(worker, store, objectStoreTestStub{}, audioProbeRunnerTestStub{}, audioNormalizeRunnerTestStub{}, activities.FakeTranscriptionProvider{}, activities.FakeSummaryProvider{})

	if got, want := len(worker.workflows), 1; got != want {
		t.Fatalf("registered workflows = %d, want %d", got, want)
	}
	if !sameFunction(worker.workflows[0], workflows.RecordingProcessingWorkflow) {
		t.Fatalf("registered workflow = %T, want RecordingProcessingWorkflow", worker.workflows[0])
	}

	wantActivityNames := []string{
		activities.ValidateRecordingActivityName,
		activities.MarkRecordingProcessingActivityName,
		activities.PrepareRecordingAudioActivityName,
		activities.MarkRecordingTranscribingActivityName,
		activities.TranscribeRecordingAudioActivityName,
		activities.MarkRecordingSummarizingActivityName,
		activities.SummarizeRecordingActivityName,
		activities.GenerateMindMapActivityName,
		activities.DeleteOriginalRecordingAudioActivityName,
		activities.CompleteRecordingProcessingActivityName,
		activities.FailRecordingProcessingActivityName,
	}
	if got, want := len(worker.activities), len(wantActivityNames); got != want {
		t.Fatalf("registered activities = %d, want %d", got, want)
	}
	for i, wantName := range wantActivityNames {
		if got := worker.activities[i].options.Name; got != wantName {
			t.Fatalf("activity %d name = %q, want %q", i, got, wantName)
		}
	}
}

func TestTranscriptionProviderForConfigDefaultsToFakeProvider(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.TranscriptionProvider = "fake_transcription"

	provider, err := transcriptionProviderForConfig(cfg)
	if err != nil {
		t.Fatalf("transcriptionProviderForConfig returned error: %v", err)
	}
	if _, ok := provider.(activities.FakeTranscriptionProvider); !ok {
		t.Fatalf("provider = %T, want FakeTranscriptionProvider", provider)
	}
}

func TestTranscriptionProviderForConfigBuildsOpenAICompatibleASR(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.TranscriptionProvider = "openai_compatible_asr"
	cfg.TranscriptionBaseURL = "http://asr.example.test/v1"
	cfg.TranscriptionAPIKey = "test-asr-key"
	cfg.TranscriptionModel = "mimo-v2.5-asr"
	cfg.TranscriptionAuthHeader = "bearer"
	cfg.TranscriptionLanguage = "zh"

	provider, err := transcriptionProviderForConfig(cfg)
	if err != nil {
		t.Fatalf("transcriptionProviderForConfig returned error: %v", err)
	}
	asr, ok := provider.(activities.OpenAICompatibleASRProvider)
	if !ok {
		t.Fatalf("provider = %T, want OpenAICompatibleASRProvider", provider)
	}
	if asr.BaseURL != cfg.TranscriptionBaseURL || asr.APIKey != cfg.TranscriptionAPIKey || asr.Model != cfg.TranscriptionModel || asr.AuthHeader != cfg.TranscriptionAuthHeader || asr.Language != cfg.TranscriptionLanguage {
		t.Fatalf("asr provider = %+v, want config-derived fields", asr)
	}
}

func TestTranscriptionProviderForConfigBuildsDashScopeASR(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.TranscriptionProvider = "dashscope_asr"
	cfg.DashScopeBaseURL = "http://dashscope.example.test/api/v1"
	cfg.DashScopeAPIKey = "dashscope-key"
	cfg.DashScopeASRModel = "paraformer-v2"
	cfg.TranscriptionLanguage = "zh"

	provider, err := transcriptionProviderForConfig(cfg)
	if err != nil {
		t.Fatalf("transcriptionProviderForConfig returned error: %v", err)
	}
	asr, ok := provider.(activities.DashScopeASRProvider)
	if !ok {
		t.Fatalf("provider = %T, want DashScopeASRProvider", provider)
	}
	if asr.BaseURL != cfg.DashScopeBaseURL || asr.APIKey != cfg.DashScopeAPIKey || asr.Model != cfg.DashScopeASRModel || asr.Language != cfg.TranscriptionLanguage {
		t.Fatalf("asr provider = %+v, want DashScope config-derived fields", asr)
	}
}

func TestTranscriptionProviderForConfigRejectsMissingAPIKeyForExternalProvider(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.TranscriptionProvider = "openai_compatible_asr"
	cfg.TranscriptionAPIKey = ""
	cfg.PrivacyAllowExternalModelProviders = true

	_, err := transcriptionProviderForConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "TRANSCRIPTION_API_KEY") {
		t.Fatalf("error = %v, want missing TRANSCRIPTION_API_KEY error", err)
	}
}

func TestTranscriptionProviderForConfigRejectsExternalProviderInPrivateMode(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.TranscriptionProvider = "openai_compatible_asr"
	cfg.TranscriptionAPIKey = "test-asr-key"
	cfg.PrivacyAllowExternalModelProviders = false

	_, err := transcriptionProviderForConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "external transcription provider") {
		t.Fatalf("error = %v, want private-mode external provider error", err)
	}
}

func TestSummaryProviderForConfigDefaultsToFakeProvider(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.LLMProvider = "fake_llm"

	provider, err := summaryProviderForConfig(cfg)
	if err != nil {
		t.Fatalf("summaryProviderForConfig returned error: %v", err)
	}
	if _, ok := provider.(activities.FakeSummaryProvider); !ok {
		t.Fatalf("provider = %T, want FakeSummaryProvider", provider)
	}
}

func TestSummaryProviderForConfigBuildsOpenAICompatibleLLM(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.LLMProvider = "openai_compatible"
	cfg.LLMBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	cfg.LLMAPIKey = "summary-key"
	cfg.LLMModel = "qwen-plus"

	provider, err := summaryProviderForConfig(cfg)
	if err != nil {
		t.Fatalf("summaryProviderForConfig returned error: %v", err)
	}
	summary, ok := provider.(activities.OpenAICompatibleSummaryProvider)
	if !ok {
		t.Fatalf("provider = %T, want OpenAICompatibleSummaryProvider", provider)
	}
	if summary.BaseURL != cfg.LLMBaseURL || summary.APIKey != cfg.LLMAPIKey || summary.Model != cfg.LLMModel {
		t.Fatalf("summary provider = %+v, want config-derived fields", summary)
	}
}

func TestSummaryProviderForConfigRejectsMissingAPIKeyForExternalProvider(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.LLMProvider = "openai_compatible"
	cfg.LLMAPIKey = ""
	cfg.PrivacyAllowExternalModelProviders = true

	_, err := summaryProviderForConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "LLM_API_KEY") {
		t.Fatalf("error = %v, want missing LLM_API_KEY error", err)
	}
}

func TestSummaryProviderForConfigRejectsExternalProviderInPrivateMode(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.LLMProvider = "openai_compatible"
	cfg.LLMAPIKey = "summary-key"
	cfg.PrivacyAllowExternalModelProviders = false

	_, err := summaryProviderForConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "external LLM provider") {
		t.Fatalf("error = %v, want private-mode external provider error", err)
	}
}

func sameFunction(a, b interface{}) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

type temporalWorkerRunnerSpy struct {
	started chan struct{}
	err     error
}

func (s *temporalWorkerRunnerSpy) Run(interruptCh <-chan interface{}) error {
	close(s.started)
	if s.err != nil {
		return s.err
	}
	<-interruptCh
	return nil
}

type backgroundRunnerSpy struct {
	started chan struct{}
	stopped chan struct{}
}

func (s *backgroundRunnerSpy) Run(ctx context.Context) {
	close(s.started)
	<-ctx.Done()
	close(s.stopped)
}

type objectStoreTestStub struct{}

func (objectStoreTestStub) PutObject(ctx context.Context, input storage.PutObjectInput) (storage.PutObjectResult, error) {
	return storage.PutObjectResult{Key: input.Key}, nil
}

func (objectStoreTestStub) GetObject(ctx context.Context, key string) (storage.GetObjectResult, error) {
	return storage.GetObjectResult{Key: key, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (objectStoreTestStub) PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return "", nil
}

func (objectStoreTestStub) DeleteObject(ctx context.Context, key string) error {
	return nil
}

type audioProbeRunnerTestStub struct{}

func (audioProbeRunnerTestStub) Probe(ctx context.Context, path string) (activities.AudioProbeResult, error) {
	return activities.AudioProbeResult{}, nil
}

type audioNormalizeRunnerTestStub struct{}

func (audioNormalizeRunnerTestStub) Normalize(ctx context.Context, input activities.AudioNormalizeRequest) (activities.AudioNormalizeResult, error) {
	return activities.AudioNormalizeResult{}, nil
}

type registeredActivity struct {
	activity interface{}
	options  activity.RegisterOptions
}

type recordingWorkerSpy struct {
	workflows  []interface{}
	activities []registeredActivity
}

func (s *recordingWorkerSpy) RegisterWorkflow(workflow interface{}) {
	s.workflows = append(s.workflows, workflow)
}

func (s *recordingWorkerSpy) RegisterActivity(activityFn interface{}) {
	s.RegisterActivityWithOptions(activityFn, activity.RegisterOptions{})
}

func (s *recordingWorkerSpy) RegisterActivityWithOptions(activityFn interface{}, options activity.RegisterOptions) {
	s.activities = append(s.activities, registeredActivity{activity: activityFn, options: options})
}

type workerRecordingStoreSpy struct{}

func (s *workerRecordingStoreSpy) Get(id string) (domain.Recording, bool, error) {
	return domain.Recording{ID: id, WorkspaceID: "wsp_default"}, true, nil
}

func (s *workerRecordingStoreSpy) GetForWorkspace(input recordings.GetRecordingInput) (domain.Recording, bool, error) {
	recording, ok, err := s.Get(input.ID)
	if err != nil {
		return domain.Recording{}, false, err
	}
	if !ok || recording.WorkspaceID != input.WorkspaceID {
		return domain.Recording{}, false, nil
	}
	return recording, true, nil
}

func (s *workerRecordingStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	return domain.Recording{ID: input.ID, WorkspaceID: input.WorkspaceID, Status: input.Status}, nil
}

func (s *workerRecordingStoreSpy) UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error) {
	return recordings.RecordingAudioProbe{RecordingID: input.RecordingID}, nil
}

func (s *workerRecordingStoreSpy) UpsertNormalizedAudio(input recordings.UpsertNormalizedAudioInput) (recordings.RecordingNormalizedAudio, error) {
	return recordings.RecordingNormalizedAudio{RecordingID: input.RecordingID, ObjectKey: input.ObjectKey}, nil
}

func (s *workerRecordingStoreSpy) GetNormalizedAudio(recordingID string) (recordings.RecordingNormalizedAudio, bool, error) {
	return recordings.RecordingNormalizedAudio{RecordingID: recordingID, ObjectKey: "recordings/rec/normalized.wav"}, true, nil
}

func (s *workerRecordingStoreSpy) UpsertTranscript(input recordings.UpsertTranscriptInput) (recordings.RecordingTranscript, error) {
	return recordings.RecordingTranscript{RecordingID: input.RecordingID}, nil
}

func (s *workerRecordingStoreSpy) GetTranscript(recordingID string) (recordings.RecordingTranscript, bool, error) {
	return recordings.RecordingTranscript{RecordingID: recordingID}, true, nil
}

func (s *workerRecordingStoreSpy) UpsertSummary(input recordings.UpsertSummaryInput) (recordings.RecordingSummary, error) {
	return recordings.RecordingSummary{RecordingID: input.RecordingID}, nil
}

func (s *workerRecordingStoreSpy) GetSummary(recordingID string) (recordings.RecordingSummary, bool, error) {
	return recordings.RecordingSummary{RecordingID: recordingID}, true, nil
}

func (s *workerRecordingStoreSpy) UpsertMindMap(input recordings.UpsertMindMapInput) (recordings.RecordingMindMap, error) {
	return recordings.RecordingMindMap{RecordingID: input.RecordingID}, nil
}
