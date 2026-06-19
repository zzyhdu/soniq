package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zzyhdu/soniq/backend/internal/activities"
	"github.com/zzyhdu/soniq/backend/internal/cleanup"
	"github.com/zzyhdu/soniq/backend/internal/config"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	"github.com/zzyhdu/soniq/backend/internal/observability"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
	"github.com/zzyhdu/soniq/backend/internal/version"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	temporalworker "go.temporal.io/sdk/worker"
)

const workerStopTimeout = 25 * time.Second

func main() {
	showVersion := flag.Bool("version", false, "print build version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Summary("soniq-worker"))
		return
	}

	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid startup config: %v\n", err)
		os.Exit(1)
	}
	logger, err := observability.NewLogger(observability.LoggerConfig{
		Service: "soniq-worker",
		Format:  cfg.LogFormat,
		Level:   cfg.LogLevel,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure logger: %v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	logger.Info("starting temporal worker",
		slog.String("event", "worker_starting"),
		slog.String("temporal_address", cfg.TemporalAddress),
		slog.String("temporal_namespace", cfg.TemporalNamespace),
		slog.String("temporal_task_queue", cfg.TemporalTaskQueue),
		slog.String("version", version.Version),
		slog.String("commit", version.Commit),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg); err != nil {
		logger.Error("worker stopped", slog.String("event", "worker_stopped"), slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config) error {
	temporalClient, err := client.DialContext(ctx, client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		return err
	}
	defer temporalClient.Close()

	recordingStore, err := openPostgresRecordingStore(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer recordingStore.Close()

	transcriptionProvider, err := transcriptionProviderForConfig(cfg)
	if err != nil {
		return err
	}
	summaryProvider, err := summaryProviderForConfig(cfg)
	if err != nil {
		return err
	}

	worker := temporalworker.New(temporalClient, cfg.TemporalTaskQueue, temporalworker.Options{
		WorkerStopTimeout: workerStopTimeout,
	})
	objectStore, err := buildObjectStore(ctx, cfg)
	if err != nil {
		return err
	}
	registerRecordingProcessing(worker, recordingStore, objectStore, activities.FFProbeRunner{}, activities.FFmpegNormalizeRunner{}, transcriptionProvider, summaryProvider)
	cleanupRunner := cleanup.NewRecordingPurgeArtifactCleaner(recordingStore, objectStore, cleanup.RecordingPurgeArtifactCleanerOptions{
		Interval:  time.Duration(cfg.PurgeArtifactCleanupIntervalSeconds) * time.Second,
		BatchSize: int(cfg.PurgeArtifactCleanupBatchSize),
		Logger:    slog.Default(),
	})
	return runTemporalWorkerWithCleanup(ctx, worker, cleanupRunner)
}

type temporalWorkerRunner interface {
	Run(<-chan interface{}) error
}

type backgroundRunner interface {
	Run(context.Context)
}

func runTemporalWorkerWithCleanup(ctx context.Context, worker temporalWorkerRunner, cleanupRunner backgroundRunner) error {
	cleanupCtx, cancelCleanup := context.WithCancel(ctx)
	defer cancelCleanup()
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		cleanupRunner.Run(cleanupCtx)
	}()

	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	err := worker.Run(interruptChFromContext(workerCtx))
	cancelCleanup()
	<-cleanupDone
	return err
}

func interruptChFromContext(ctx context.Context) <-chan interface{} {
	interruptCh := make(chan interface{})
	go func() {
		<-ctx.Done()
		close(interruptCh)
	}()
	return interruptCh
}

type recordingProcessingRegistry interface {
	RegisterWorkflow(interface{})
	RegisterActivity(interface{})
	RegisterActivityWithOptions(interface{}, activity.RegisterOptions)
}

func registerRecordingProcessing(registry recordingProcessingRegistry, store activities.NormalizingPipelineStore, objectStore storage.ObjectStore, probeRunner activities.AudioProbeRunner, normalizeRunner activities.AudioNormalizeRunner, transcriptionProvider activities.TranscriptionProvider, summaryProvider activities.SummaryProvider) {
	activitySet := activities.NewRecordingProcessingActivitiesWithNormalizedAudio(
		store,
		objectStore,
		probeRunner,
		normalizeRunner,
		transcriptionProvider,
		summaryProvider,
	)

	registry.RegisterWorkflow(workflows.RecordingProcessingWorkflow)
	registry.RegisterActivityWithOptions(activitySet.ValidateRecording, activity.RegisterOptions{Name: activities.ValidateRecordingActivityName})
	registry.RegisterActivityWithOptions(activitySet.MarkRecordingProcessing, activity.RegisterOptions{Name: activities.MarkRecordingProcessingActivityName})
	registry.RegisterActivityWithOptions(activitySet.PrepareRecordingAudio, activity.RegisterOptions{Name: activities.PrepareRecordingAudioActivityName})
	registry.RegisterActivityWithOptions(activitySet.MarkRecordingTranscribing, activity.RegisterOptions{Name: activities.MarkRecordingTranscribingActivityName})
	registry.RegisterActivityWithOptions(activitySet.TranscribeRecordingAudio, activity.RegisterOptions{Name: activities.TranscribeRecordingAudioActivityName})
	registry.RegisterActivityWithOptions(activitySet.MarkRecordingSummarizing, activity.RegisterOptions{Name: activities.MarkRecordingSummarizingActivityName})
	registry.RegisterActivityWithOptions(activitySet.SummarizeRecording, activity.RegisterOptions{Name: activities.SummarizeRecordingActivityName})
	registry.RegisterActivityWithOptions(activitySet.GenerateMindMap, activity.RegisterOptions{Name: activities.GenerateMindMapActivityName})
	registry.RegisterActivityWithOptions(activitySet.DeleteOriginalRecordingAudio, activity.RegisterOptions{Name: activities.DeleteOriginalRecordingAudioActivityName})
	registry.RegisterActivityWithOptions(activitySet.CompleteRecordingProcessing, activity.RegisterOptions{Name: activities.CompleteRecordingProcessingActivityName})
	registry.RegisterActivityWithOptions(activitySet.FailRecordingProcessing, activity.RegisterOptions{Name: activities.FailRecordingProcessingActivityName})
}

func transcriptionProviderForConfig(cfg config.Config) (activities.TranscriptionProvider, error) {
	provider := strings.TrimSpace(cfg.TranscriptionProvider)
	switch provider {
	case "", "fake_transcription":
		return activities.FakeTranscriptionProvider{}, nil
	case "openai_compatible_asr":
		if !cfg.PrivacyAllowExternalModelProviders {
			return nil, fmt.Errorf("external transcription provider %q is disabled by privacy settings", provider)
		}
		if strings.TrimSpace(cfg.TranscriptionAPIKey) == "" {
			return nil, fmt.Errorf("TRANSCRIPTION_API_KEY is required for external transcription provider %q", provider)
		}
		if strings.TrimSpace(cfg.TranscriptionBaseURL) == "" {
			return nil, fmt.Errorf("TRANSCRIPTION_BASE_URL is required for external transcription provider %q", provider)
		}
		if strings.TrimSpace(cfg.TranscriptionModel) == "" {
			return nil, fmt.Errorf("TRANSCRIPTION_MODEL is required for external transcription provider %q", provider)
		}
		return activities.OpenAICompatibleASRProvider{
			BaseURL:    cfg.TranscriptionBaseURL,
			APIKey:     cfg.TranscriptionAPIKey,
			Model:      cfg.TranscriptionModel,
			AuthHeader: cfg.TranscriptionAuthHeader,
			Language:   cfg.TranscriptionLanguage,
		}, nil
	case "dashscope_asr":
		if !cfg.PrivacyAllowExternalModelProviders {
			return nil, fmt.Errorf("external transcription provider %q is disabled by privacy settings", provider)
		}
		if strings.TrimSpace(cfg.DashScopeAPIKey) == "" {
			return nil, fmt.Errorf("DASHSCOPE_API_KEY is required for external transcription provider %q", provider)
		}
		if strings.TrimSpace(cfg.DashScopeBaseURL) == "" {
			return nil, fmt.Errorf("DASHSCOPE_BASE_URL is required for external transcription provider %q", provider)
		}
		if strings.TrimSpace(cfg.DashScopeASRModel) == "" {
			return nil, fmt.Errorf("DASHSCOPE_ASR_MODEL is required for external transcription provider %q", provider)
		}
		return activities.DashScopeASRProvider{
			BaseURL:     cfg.DashScopeBaseURL,
			APIKey:      cfg.DashScopeAPIKey,
			Model:       cfg.DashScopeASRModel,
			Language:    cfg.TranscriptionLanguage,
			Diarization: true,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported transcription provider %q", provider)
	}
}

func summaryProviderForConfig(cfg config.Config) (activities.SummaryProvider, error) {
	provider := strings.TrimSpace(cfg.LLMProvider)
	switch provider {
	case "", "fake_llm":
		return activities.FakeSummaryProvider{}, nil
	case "openai_compatible":
		if !cfg.PrivacyAllowExternalModelProviders {
			return nil, fmt.Errorf("external LLM provider %q is disabled by privacy settings", provider)
		}
		if strings.TrimSpace(cfg.LLMAPIKey) == "" {
			return nil, fmt.Errorf("LLM_API_KEY is required for external LLM provider %q", provider)
		}
		if strings.TrimSpace(cfg.LLMBaseURL) == "" {
			return nil, fmt.Errorf("LLM_BASE_URL is required for external LLM provider %q", provider)
		}
		if strings.TrimSpace(cfg.LLMModel) == "" {
			return nil, fmt.Errorf("LLM_MODEL is required for external LLM provider %q", provider)
		}
		return activities.OpenAICompatibleSummaryProvider{
			BaseURL: cfg.LLMBaseURL,
			APIKey:  cfg.LLMAPIKey,
			Model:   cfg.LLMModel,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider %q", provider)
	}
}

func buildObjectStore(ctx context.Context, cfg config.Config) (storage.ObjectStore, error) {
	return storage.NewObjectStore(ctx, storage.ProviderConfig{
		Provider:         cfg.StorageProvider,
		S3Endpoint:       cfg.S3Endpoint,
		S3Region:         cfg.S3Region,
		S3Bucket:         cfg.S3Bucket,
		S3AccessKey:      cfg.S3AccessKey,
		S3SecretKey:      cfg.S3SecretKey,
		S3ForcePathStyle: cfg.S3ForcePathStyle,
	})
}

type recordingStoreClient interface {
	activities.NormalizingPipelineStore
	cleanup.RecordingPurgeArtifactStore
	Close()
}

func openPostgresRecordingStore(ctx context.Context, dsn string) (recordingStoreClient, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &postgresRecordingStoreClient{
		PostgresStore: recordings.NewPostgresStore(storedb.NewPgxPoolExecutor(pool)),
		pool:          pool,
	}, nil
}

type postgresRecordingStoreClient struct {
	*recordings.PostgresStore
	pool *pgxpool.Pool
}

func (s *postgresRecordingStoreClient) Close() {
	s.pool.Close()
}
