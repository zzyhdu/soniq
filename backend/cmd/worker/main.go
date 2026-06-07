package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zzyhdu/soniq/backend/internal/activities"
	"github.com/zzyhdu/soniq/backend/internal/config"
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

	worker := temporalworker.New(temporalClient, cfg.TemporalTaskQueue, temporalworker.Options{})
	registerRecordingProcessing(worker, recordingStore, storage.NewLocalStore(cfg.LocalStoragePath), activities.FFProbeRunner{}, activities.FFmpegNormalizeRunner{}, transcriptionProvider)

	return worker.Run(temporalworker.InterruptCh())
}

type recordingProcessingRegistry interface {
	RegisterWorkflow(interface{})
	RegisterActivity(interface{})
	RegisterActivityWithOptions(interface{}, activity.RegisterOptions)
}

func registerRecordingProcessing(registry recordingProcessingRegistry, store activities.NormalizingPipelineStore, resolver activities.LocalObjectPathResolver, probeRunner activities.AudioProbeRunner, normalizeRunner activities.AudioNormalizeRunner, transcriptionProvider activities.TranscriptionProvider) {
	activitySet := activities.NewRecordingProcessingActivitiesWithNormalizedAudio(
		store,
		resolver,
		probeRunner,
		normalizeRunner,
		transcriptionProvider,
		activities.FakeSummaryProvider{},
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

type recordingStoreClient interface {
	activities.NormalizingPipelineStore
	Close()
}

func openPostgresRecordingStore(ctx context.Context, dsn string) (recordingStoreClient, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &postgresRecordingStoreClient{
		PostgresStore: recordings.NewPostgresStore(postgresExecutor{pool: pool}),
		pool:          pool,
	}, nil
}

type postgresExecutor struct {
	pool *pgxpool.Pool
}

func (e postgresExecutor) QueryRow(ctx context.Context, query string, args ...any) interface{ Scan(dest ...any) error } {
	return e.pool.QueryRow(ctx, query, args...)
}

type postgresRecordingStoreClient struct {
	*recordings.PostgresStore
	pool *pgxpool.Pool
}

func (s *postgresRecordingStoreClient) Close() {
	s.pool.Close()
}
