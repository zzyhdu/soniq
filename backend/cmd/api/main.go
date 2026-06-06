package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zzyhdu/soniq/backend/internal/api"
	"github.com/zzyhdu/soniq/backend/internal/config"
	"github.com/zzyhdu/soniq/backend/internal/processing"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
	"go.temporal.io/sdk/client"
)

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		log.Fatalf("invalid startup config: %v", err)
	}

	handler, cleanup, err := buildHandler(context.Background(), cfg, dialTemporalClient, openPostgresRecordingStore)
	if err != nil {
		log.Fatalf("build api handler: %v", err)
	}
	defer cleanup()

	addr := cfg.APIAddress
	log.Printf("starting soniq-api on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("api server stopped: %v", err)
	}
}

type temporalWorkflowClient interface {
	processing.WorkflowStarter
	Close()
}

type temporalClientFactory func(context.Context, config.Config) (temporalWorkflowClient, error)

type recordingStoreClient interface {
	api.RecordingStore
	Close()
}

type recordingStoreFactory func(context.Context, string) (recordingStoreClient, error)

func buildHandler(ctx context.Context, cfg config.Config, temporalFactory temporalClientFactory, storeFactory recordingStoreFactory) (http.Handler, func(), error) {
	temporalClient, err := temporalFactory(ctx, cfg)
	if err != nil {
		return nil, func() {}, err
	}
	recordingStore, err := storeFactory(ctx, cfg.PostgresDSN)
	if err != nil {
		temporalClient.Close()
		return nil, func() {}, err
	}

	processor := processing.NewTemporalRecordingProcessor(temporalClient, processing.TemporalRecordingProcessorConfig{
		TaskQueue: cfg.TemporalTaskQueue,
	})
	objectStore, err := buildObjectStore(cfg)
	if err != nil {
		recordingStore.Close()
		temporalClient.Close()
		return nil, func() {}, err
	}
	handler := api.NewRouterWithStorage(recordingStore, processor, objectStore)
	cleanup := func() {
		recordingStore.Close()
		temporalClient.Close()
	}

	return handler, cleanup, nil
}

func buildObjectStore(cfg config.Config) (storage.ObjectStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.StorageProvider)) {
	case "local":
		return storage.NewLocalStore(cfg.LocalStoragePath), nil
	default:
		return nil, fmt.Errorf("unsupported storage provider %q", cfg.StorageProvider)
	}
}

func dialTemporalClient(ctx context.Context, cfg config.Config) (temporalWorkflowClient, error) {
	return client.DialContext(ctx, client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
	})
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
