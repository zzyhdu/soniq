package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zzyhdu/soniq/backend/internal/activities"
	"github.com/zzyhdu/soniq/backend/internal/cleanup"
	"github.com/zzyhdu/soniq/backend/internal/config"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	temporalworker "go.temporal.io/sdk/worker"
)

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		log.Fatalf("invalid startup config: %v", err)
	}

	log.Printf("starting temporal worker")
	log.Printf("temporal_address=%s", cfg.TemporalAddress)
	log.Printf("temporal_namespace=%s", cfg.TemporalNamespace)
	log.Printf("temporal_task_queue=%s", cfg.TemporalTaskQueue)

	if err := run(context.Background(), cfg); err != nil {
		log.Fatalf("worker stopped: %v", err)
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

	worker := temporalworker.New(temporalClient, cfg.TemporalTaskQueue, temporalworker.Options{})
	objectStore := storage.NewLocalStore(cfg.LocalStoragePath)
	registerRecordingProcessing(worker, recordingStore, objectStore, objectStore, activities.FFProbeRunner{}, activities.FFmpegNormalizeRunner{}, transcriptionProvider, summaryProvider)
	cleanupCtx, cancelCleanup := context.WithCancel(ctx)
	defer cancelCleanup()
	cleanupRunner := cleanup.NewRecordingPurgeArtifactCleaner(recordingStore, objectStore, cleanup.RecordingPurgeArtifactCleanerOptions{
		Interval:  time.Duration(cfg.PurgeArtifactCleanupIntervalSeconds) * time.Second,
		BatchSize: int(cfg.PurgeArtifactCleanupBatchSize),
	})
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		cleanupRunner.Run(cleanupCtx)
	}()

	err = worker.Run(temporalworker.InterruptCh())
	cancelCleanup()
	<-cleanupDone
	return err
}

type recordingProcessingRegistry interface {
	RegisterWorkflow(interface{})
	RegisterActivity(interface{})
	RegisterActivityWithOptions(interface{}, activity.RegisterOptions)
}

func registerRecordingProcessing(registry recordingProcessingRegistry, store activities.NormalizingPipelineStore, resolver activities.LocalObjectPathResolver, objectStore storage.ObjectStore, probeRunner activities.AudioProbeRunner, normalizeRunner activities.AudioNormalizeRunner, transcriptionProvider activities.TranscriptionProvider, summaryProvider activities.SummaryProvider) {
	activitySet := activities.NewRecordingProcessingActivitiesWithNormalizedAudio(
		store,
		resolver,
		objectStore,
		probeRunner,
		normalizeRunner,
		transcriptionProvider,
		summaryProvider,
	)

	registry.RegisterWorkflow(workflows.RecordingProcessingWorkflow)
	registry.RegisterActivityWithOptions(activitySet.ValidateRecording, activity.RegisterOptions{Name: activities.ValidateRecordingActivityName})
	registry.RegisterActivityWithOptions(activitySet.MarkRecordingProcessing, activity.RegisterOptions{Name: activities.MarkRecordingProcessingActivityName})
	registry.RegisterActivityWithOptions(activitySet.ProbeRecordingAudio, activity.RegisterOptions{Name: activities.ProbeRecordingAudioActivityName})
	registry.RegisterActivityWithOptions(activitySet.NormalizeRecordingAudio, activity.RegisterOptions{Name: activities.NormalizeRecordingAudioActivityName})
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
			BaseURL:        cfg.TranscriptionBaseURL,
			APIKey:         cfg.TranscriptionAPIKey,
			Model:          cfg.TranscriptionModel,
			AuthHeader:     cfg.TranscriptionAuthHeader,
			Language:       cfg.TranscriptionLanguage,
			MaxBase64Bytes: cfg.TranscriptionMaxBase64Bytes,
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
			BaseURL:        cfg.DashScopeBaseURL,
			APIKey:         cfg.DashScopeAPIKey,
			Model:          cfg.DashScopeASRModel,
			Language:       cfg.TranscriptionLanguage,
			Diarization:    true,
			MaxBase64Bytes: cfg.TranscriptionMaxBase64Bytes,
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
		PostgresStore: recordings.NewPostgresStore(postgresExecutor{pool: pool}),
		pool:          pool,
	}, nil
}

type postgresExecutor struct {
	pool *pgxpool.Pool
}

func (e postgresExecutor) QueryRow(ctx context.Context, query string, args ...any) storedb.PostgresRow {
	return e.pool.QueryRow(ctx, query, args...)
}

func (e postgresExecutor) Query(ctx context.Context, query string, args ...any) (storedb.PostgresRows, error) {
	return e.pool.Query(ctx, query, args...)
}

func (e postgresExecutor) Begin(ctx context.Context) (storedb.PostgresTx, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return postgresTx{tx: tx}, nil
}

type postgresTx struct {
	tx pgx.Tx
}

func (t postgresTx) QueryRow(ctx context.Context, query string, args ...any) storedb.PostgresRow {
	return t.tx.QueryRow(ctx, query, args...)
}

func (t postgresTx) Query(ctx context.Context, query string, args ...any) (storedb.PostgresRows, error) {
	return t.tx.Query(ctx, query, args...)
}

func (t postgresTx) Exec(ctx context.Context, query string, args ...any) error {
	_, err := t.tx.Exec(ctx, query, args...)
	return err
}

func (t postgresTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t postgresTx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}

type postgresRecordingStoreClient struct {
	*recordings.PostgresStore
	pool *pgxpool.Pool
}

func (s *postgresRecordingStoreClient) Close() {
	s.pool.Close()
}
