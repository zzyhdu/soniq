package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/activities"
	"github.com/zzyhdu/soniq/backend/internal/config"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/observability"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

func TestWorkerOptionsForConfigMapsConcurrencyLimits(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.WorkerMaxConcurrentWorkflowTasks = 32
	cfg.WorkerMaxConcurrentActivities = 6
	cfg.WorkerMaxConcurrentLocalActivities = 5
	cfg.WorkerTaskQueueActivitiesPerSecond = 2.5

	options := workerOptionsForConfig(cfg)

	if options.WorkerStopTimeout != workerStopTimeout {
		t.Fatalf("WorkerStopTimeout = %s, want %s", options.WorkerStopTimeout, workerStopTimeout)
	}
	if options.MaxConcurrentWorkflowTaskExecutionSize != 32 {
		t.Fatalf("MaxConcurrentWorkflowTaskExecutionSize = %d, want 32", options.MaxConcurrentWorkflowTaskExecutionSize)
	}
	if options.MaxConcurrentActivityExecutionSize != 6 {
		t.Fatalf("MaxConcurrentActivityExecutionSize = %d, want 6", options.MaxConcurrentActivityExecutionSize)
	}
	if options.MaxConcurrentLocalActivityExecutionSize != 5 {
		t.Fatalf("MaxConcurrentLocalActivityExecutionSize = %d, want 5", options.MaxConcurrentLocalActivityExecutionSize)
	}
	if options.TaskQueueActivitiesPerSecond != 2.5 {
		t.Fatalf("TaskQueueActivitiesPerSecond = %f, want 2.5", options.TaskQueueActivitiesPerSecond)
	}
}

func TestTemporalClientOptionsForConfigIncludesSDKMetricsHandler(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.TemporalAddress = "temporal.example.test:7233"
	cfg.TemporalNamespace = "default"
	cfg.TemporalTaskQueue = "soniq-audio-pipeline"
	metrics := observability.NewMetrics()

	options, err := temporalClientOptionsForConfig(cfg, metrics, nil)
	if err != nil {
		t.Fatalf("temporalClientOptionsForConfig() error = %v, want nil", err)
	}

	if options.HostPort != cfg.TemporalAddress {
		t.Fatalf("HostPort = %q, want %q", options.HostPort, cfg.TemporalAddress)
	}
	if options.Namespace != cfg.TemporalNamespace {
		t.Fatalf("Namespace = %q, want %q", options.Namespace, cfg.TemporalNamespace)
	}
	if options.MetricsHandler == nil {
		t.Fatal("MetricsHandler is nil, want Temporal SDK metrics handler")
	}
	options.MetricsHandler.WithTags(map[string]string{
		"namespace":  cfg.TemporalNamespace,
		"task_queue": cfg.TemporalTaskQueue,
	}).Counter("temporal_worker_start").Inc(1)

	body := workerMetricsBody(t, metrics)
	for _, want := range []string{
		`temporal_worker_start{`,
		`namespace="default"`,
		`task_queue="soniq-audio-pipeline"`,
		`} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, body)
		}
	}
}

func TestTemporalClientOptionsForConfigAddsTracingInterceptorWhenEnabled(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.TemporalAddress = "temporal.example.test:7233"
	cfg.TemporalNamespace = "default"
	tracing, err := observability.NewTracing(context.Background(), observability.TracingConfig{
		Enabled:     true,
		ServiceName: "soniq-worker",
		Environment: "test",
	})
	if err != nil {
		t.Fatalf("NewTracing() error = %v, want nil", err)
	}
	defer tracing.Shutdown(context.Background()) //nolint:errcheck

	options, err := temporalClientOptionsForConfig(cfg, nil, tracing)
	if err != nil {
		t.Fatalf("temporalClientOptionsForConfig() error = %v, want nil", err)
	}
	if got, want := len(options.Interceptors), 1; got != want {
		t.Fatalf("Interceptors = %d, want %d", got, want)
	}
}

func TestRegisterRecordingProcessingRegistersWorkflowAndActivities(t *testing.T) {
	worker := &recordingWorkerSpy{}
	store := &workerRecordingStoreSpy{}

	registerRecordingProcessing(worker, store, objectStoreTestStub{}, audioProbeRunnerTestStub{}, audioNormalizeRunnerTestStub{}, activities.FakeTranscriptionProvider{}, activities.FakeSummaryProvider{}, nil)

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

func TestRegisterRecordingProcessingRecordsActivityMetrics(t *testing.T) {
	worker := &recordingWorkerSpy{}
	store := &workerRecordingStoreSpy{}
	metrics := observability.NewMetrics()

	registerRecordingProcessing(worker, store, objectStoreTestStub{}, audioProbeRunnerTestStub{}, audioNormalizeRunnerTestStub{}, activities.FakeTranscriptionProvider{}, activities.FakeSummaryProvider{}, metrics)

	completeActivity := findRegisteredActivity[func(context.Context, activities.RecordingReference) (activities.RecordingProcessingResult, error)](t, worker, activities.CompleteRecordingProcessingActivityName)
	if _, err := completeActivity(context.Background(), activities.RecordingReference{WorkspaceID: "wsp_default", RecordingID: "rec_1"}); err != nil {
		t.Fatalf("complete activity returned error: %v", err)
	}
	failActivity := findRegisteredActivity[func(context.Context, activities.RecordingFailure) error](t, worker, activities.FailRecordingProcessingActivityName)
	if err := failActivity(context.Background(), activities.RecordingFailure{WorkspaceID: "wsp_default", RecordingID: "rec_2", Reason: "transcribe audio: failed"}); err != nil {
		t.Fatalf("fail activity returned error: %v", err)
	}

	body := workerMetricsBody(t, metrics)
	for _, want := range []string{
		`soniq_worker_activities_total{activity="` + activities.CompleteRecordingProcessingActivityName + `",result="success"} 1`,
		`soniq_worker_activities_total{activity="` + activities.FailRecordingProcessingActivityName + `",result="success"} 1`,
		`soniq_recording_terminal_status_updates_total{status="completed"} 1`,
		`soniq_recording_terminal_status_updates_total{status="failed"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "rec_1") || strings.Contains(body, "rec_2") || strings.Contains(body, "wsp_default") {
		t.Fatalf("metrics output leaked high-cardinality IDs:\n%s", body)
	}
}

func TestRecordActivityAnnotatesCurrentSpan(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	defer tracerProvider.Shutdown(context.Background()) //nolint:errcheck
	ctx, span := tracerProvider.Tracer("test").Start(context.Background(), "RunActivity:ValidateRecordingActivity")
	wrapped := recordActivity(nil, activities.ValidateRecordingActivityName, func(context.Context, activities.RecordingProcessingInput) error {
		return nil
	})

	if err := wrapped(ctx, activities.RecordingProcessingInput{
		WorkspaceID: "wsp_1",
		RecordingID: "rec_1",
	}); err != nil {
		t.Fatalf("wrapped activity error = %v, want nil", err)
	}
	span.End()

	spans := spanRecorder.Ended()
	if got, want := len(spans), 1; got != want {
		t.Fatalf("ended spans = %d, want %d", got, want)
	}
	attrs := spans[0].Attributes()
	assertTraceAttribute(t, attrs, "activity", activities.ValidateRecordingActivityName)
	assertTraceAttribute(t, attrs, "workspace_id", "wsp_1")
	assertTraceAttribute(t, attrs, "recording_id", "rec_1")
}

func TestRegisterRecordingProcessingDoesNotRecordFailedOutcomeWhenFailureStatusWriteFails(t *testing.T) {
	worker := &recordingWorkerSpy{}
	store := &workerRecordingStoreSpy{updateStatusErr: errors.New("update status failed")}
	metrics := observability.NewMetrics()

	registerRecordingProcessing(worker, store, objectStoreTestStub{}, audioProbeRunnerTestStub{}, audioNormalizeRunnerTestStub{}, activities.FakeTranscriptionProvider{}, activities.FakeSummaryProvider{}, metrics)

	failActivity := findRegisteredActivity[func(context.Context, activities.RecordingFailure) error](t, worker, activities.FailRecordingProcessingActivityName)
	err := failActivity(context.Background(), activities.RecordingFailure{WorkspaceID: "wsp_default", RecordingID: "rec_2", Reason: "transcribe audio: failed"})
	if err == nil {
		t.Fatal("fail activity returned nil error, want update status error")
	}

	body := workerMetricsBody(t, metrics)
	if !strings.Contains(body, `soniq_worker_activities_total{activity="`+activities.FailRecordingProcessingActivityName+`",result="error"} 1`) {
		t.Fatalf("metrics output missing failed activity execution:\n%s", body)
	}
	if strings.Contains(body, `soniq_recording_terminal_status_updates_total{status="failed"}`) {
		t.Fatalf("metrics output recorded failed processing outcome before failure status write succeeded:\n%s", body)
	}
}

func TestStartWorkerMetricsServerServesMetrics(t *testing.T) {
	metrics := observability.NewMetrics()
	metrics.ObserveRecordingTerminalStatus(observability.MetricsRecordingStatusCompleted)

	address, stop, err := startWorkerMetricsServer(context.Background(), "127.0.0.1:0", metrics, nil)
	if err != nil {
		t.Fatalf("startWorkerMetricsServer returned error: %v", err)
	}
	defer stop()

	response, err := http.Get("http://" + address + "/metrics")
	if err != nil {
		t.Fatalf("GET worker metrics: %v", err)
	}
	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	body := string(bodyBytes)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200; body=%s", response.StatusCode, body)
	}
	if !strings.Contains(body, `soniq_recording_terminal_status_updates_total{status="completed"} 1`) {
		t.Fatalf("metrics output missing recording terminal status counter:\n%s", body)
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

func findRegisteredActivity[T any](t *testing.T, worker *recordingWorkerSpy, name string) T {
	t.Helper()
	for _, registered := range worker.activities {
		if registered.options.Name == name {
			activityFn, ok := registered.activity.(T)
			if !ok {
				t.Fatalf("activity %s type = %T, want requested activity type", name, registered.activity)
			}
			return activityFn
		}
	}
	t.Fatalf("activity %s was not registered", name)
	var zero T
	return zero
}

func workerMetricsBody(t *testing.T, metrics *observability.Metrics) string {
	t.Helper()
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", response.Code)
	}
	return response.Body.String()
}

func assertTraceAttribute(t *testing.T, attrs []attribute.KeyValue, key string, want string) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			if got := attr.Value.AsString(); got != want {
				t.Fatalf("%s = %q, want %q", key, got, want)
			}
			return
		}
	}
	t.Fatalf("attribute %q missing from %#v", key, attrs)
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

type workerRecordingStoreSpy struct {
	updateStatusErr error
}

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
	if s.updateStatusErr != nil {
		return domain.Recording{}, s.updateStatusErr
	}
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
