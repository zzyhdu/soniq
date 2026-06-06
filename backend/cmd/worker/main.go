package main

import (
	"context"
	"log"

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

	worker := temporalworker.New(temporalClient, cfg.TemporalTaskQueue, temporalworker.Options{})
	registerRecordingProcessing(worker, recordingStore, storage.NewLocalStore(cfg.LocalStoragePath), activities.FFProbeRunner{})

	return worker.Run(temporalworker.InterruptCh())
}

type recordingProcessingRegistry interface {
	RegisterWorkflow(interface{})
	RegisterActivity(interface{})
	RegisterActivityWithOptions(interface{}, activity.RegisterOptions)
}

func registerRecordingProcessing(registry recordingProcessingRegistry, store activities.RecordingStore, resolver activities.LocalObjectPathResolver, runner activities.AudioProbeRunner) {
	activitySet := activities.NewRecordingProcessingActivitiesWithAudioProbe(store, resolver, runner)

	registry.RegisterWorkflow(workflows.RecordingProcessingWorkflow)
	registry.RegisterActivityWithOptions(activitySet.ValidateRecording, activity.RegisterOptions{Name: "ValidateRecordingActivity"})
	registry.RegisterActivityWithOptions(activitySet.MarkRecordingProcessing, activity.RegisterOptions{Name: "MarkRecordingProcessingActivity"})
	registry.RegisterActivityWithOptions(activitySet.ProbeRecordingAudio, activity.RegisterOptions{Name: "ProbeRecordingAudioActivity"})
	registry.RegisterActivityWithOptions(activitySet.CompleteRecordingProcessing, activity.RegisterOptions{Name: "CompleteRecordingProcessingActivity"})
	registry.RegisterActivityWithOptions(activitySet.FailRecordingProcessing, activity.RegisterOptions{Name: "FailRecordingProcessingActivity"})
}

type recordingStoreClient interface {
	activities.RecordingStore
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
