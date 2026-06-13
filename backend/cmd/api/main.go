package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zzyhdu/soniq/backend/internal/api"
	authstore "github.com/zzyhdu/soniq/backend/internal/auth"
	"github.com/zzyhdu/soniq/backend/internal/config"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	"github.com/zzyhdu/soniq/backend/internal/processing"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
	"github.com/zzyhdu/soniq/backend/internal/workspaces"
	"go.temporal.io/sdk/client"
)

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		log.Fatalf("invalid startup config: %v", err)
	}

	handler, cleanup, err := buildHandler(context.Background(), cfg, dialTemporalClient, openPostgresAppStore)
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

type appStoreClient interface {
	RecordingStore() api.RecordingDetailsStore
	WorkspaceStore() api.WorkspaceStore
	AuthStore() appAuthStore
	Close()
}

type appAuthStore interface {
	api.PasswordAuthStore
	api.PasswordSessionStore
	api.SessionLookupStore
}

type appStoreFactory func(context.Context, string) (appStoreClient, error)

func buildHandler(ctx context.Context, cfg config.Config, temporalFactory temporalClientFactory, storeFactory appStoreFactory) (http.Handler, func(), error) {
	temporalClient, err := temporalFactory(ctx, cfg)
	if err != nil {
		return nil, func() {}, err
	}
	appStore, err := storeFactory(ctx, cfg.PostgresDSN)
	if err != nil {
		temporalClient.Close()
		return nil, func() {}, err
	}

	processor := processing.NewTemporalRecordingProcessor(temporalClient, processing.TemporalRecordingProcessorConfig{
		TaskQueue:                             cfg.TemporalTaskQueue,
		DeleteOriginalAudioAfterTranscription: cfg.PrivacyDeleteOriginalAudioAfterTranscription,
	})
	objectStore, err := buildObjectStore(cfg)
	if err != nil {
		appStore.Close()
		temporalClient.Close()
		return nil, func() {}, err
	}
	authResolver, passwordAuthConfig, err := buildAuthDependencies(cfg, appStore)
	if err != nil {
		appStore.Close()
		temporalClient.Close()
		return nil, func() {}, err
	}
	handler := api.NewRouterWithStorageIdentityAndPasswordAuth(appStore.RecordingStore(), appStore.WorkspaceStore(), authResolver, processor, objectStore, passwordAuthConfig)
	cleanup := func() {
		appStore.Close()
		temporalClient.Close()
	}

	return handler, cleanup, nil
}

func buildAuthDependencies(cfg config.Config, appStore appStoreClient) (api.AuthResolver, api.PasswordAuthConfig, error) {
	authStore := appStore.AuthStore()
	if authStore == nil {
		return nil, api.PasswordAuthConfig{}, fmt.Errorf("password auth store is not configured")
	}
	sessionTTL := time.Duration(cfg.AuthSessionTTLHours) * time.Hour
	if sessionTTL <= 0 {
		sessionTTL = 30 * 24 * time.Hour
	}
	return api.NewSessionAuthResolver(authStore), api.PasswordAuthConfig{
		PasswordStore: authStore,
		SessionStore:  authStore,
		SessionTTL:    sessionTTL,
		CookieSecure:  cfg.AuthCookieSecure,
	}, nil
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

func openPostgresAppStore(ctx context.Context, dsn string) (appStoreClient, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	executor := postgresExecutor{pool: pool}
	return &postgresAppStoreClient{
		recordings: recordings.NewPostgresStore(executor),
		workspaces: workspaces.NewPostgresStore(executor),
		auth:       authstore.NewPostgresStore(executor),
		pool:       pool,
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

type postgresAppStoreClient struct {
	recordings *recordings.PostgresStore
	workspaces *workspaces.PostgresStore
	auth       *authstore.PostgresStore
	pool       *pgxpool.Pool
}

func (s *postgresAppStoreClient) RecordingStore() api.RecordingDetailsStore {
	return s.recordings
}

func (s *postgresAppStoreClient) WorkspaceStore() api.WorkspaceStore {
	return s.workspaces
}

func (s *postgresAppStoreClient) AuthStore() appAuthStore {
	return s.auth
}

func (s *postgresAppStoreClient) Close() {
	s.pool.Close()
}
