package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zzyhdu/soniq/backend/internal/api"
	authstore "github.com/zzyhdu/soniq/backend/internal/auth"
	"github.com/zzyhdu/soniq/backend/internal/config"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	"github.com/zzyhdu/soniq/backend/internal/observability"
	"github.com/zzyhdu/soniq/backend/internal/processing"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
	"github.com/zzyhdu/soniq/backend/internal/version"
	"github.com/zzyhdu/soniq/backend/internal/workspaces"
	"go.temporal.io/sdk/client"
)

const (
	requiredSchemaMigrationVersion = 6
	readinessCheckTimeout          = 2 * time.Second
)

func main() {
	showVersion := flag.Bool("version", false, "print build version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Summary("soniq-api"))
		return
	}

	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid startup config: %v\n", err)
		os.Exit(1)
	}
	logger, err := observability.NewLogger(observability.LoggerConfig{
		Service: "soniq-api",
		Format:  cfg.LogFormat,
		Level:   cfg.LogLevel,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure logger: %v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	handler, cleanup, err := buildHandler(context.Background(), cfg, dialTemporalClient, openPostgresAppStore)
	if err != nil {
		logger.Error("build api handler", slog.String("event", "api_startup_failed"), slog.Any("error", err))
		os.Exit(1)
	}
	defer cleanup()

	addr := cfg.APIAddress
	logger.Info("starting soniq-api",
		slog.String("event", "api_starting"),
		slog.String("address", addr),
		slog.String("version", version.Version),
		slog.String("commit", version.Commit),
	)
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Error("api server stopped", slog.String("event", "api_stopped"), slog.Any("error", err))
		os.Exit(1)
	}
}

type temporalWorkflowClient interface {
	processing.WorkflowStarter
	CheckHealth(ctx context.Context, request *client.CheckHealthRequest) (*client.CheckHealthResponse, error)
	Close()
}

type temporalClientFactory func(context.Context, config.Config) (temporalWorkflowClient, error)

type appStoreClient interface {
	RecordingStore() api.RecordingStore
	WorkspaceStore() api.WorkspaceStore
	AuthStore() appAuthStore
	Ping(ctx context.Context) error
	LatestSchemaMigrationVersion(ctx context.Context) (int, error)
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
	objectStore, err := buildObjectStore(ctx, cfg)
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
	readinessChecker := apiReadinessChecker{
		appStore:                       appStore,
		temporalClient:                 temporalClient,
		objectStore:                    objectStore,
		storageProvider:                cfg.StorageProvider,
		requiredSchemaMigrationVersion: requiredSchemaMigrationVersion,
	}
	handler := api.NewRouterWithStorageIdentityPasswordAuthAndReadiness(appStore.RecordingStore(), appStore.WorkspaceStore(), authResolver, processor, objectStore, passwordAuthConfig, readinessChecker)
	cleanup := func() {
		appStore.Close()
		temporalClient.Close()
	}

	return handler, cleanup, nil
}

type apiReadinessChecker struct {
	appStore                       appStoreClient
	temporalClient                 temporalWorkflowClient
	objectStore                    storage.ObjectStore
	storageProvider                string
	requiredSchemaMigrationVersion int
}

func (c apiReadinessChecker) CheckReadiness(ctx context.Context) api.ReadinessReport {
	checkCtx, cancel := context.WithTimeout(ctx, readinessCheckTimeout)
	defer cancel()

	return api.ReadinessReport{Checks: map[string]api.ReadinessCheck{
		"postgres":       c.checkPostgres(checkCtx),
		"migrations":     c.checkMigrations(checkCtx),
		"temporal":       c.checkTemporal(checkCtx),
		"object_storage": c.checkObjectStorage(checkCtx),
	}}
}

func (c apiReadinessChecker) checkPostgres(ctx context.Context) api.ReadinessCheck {
	if c.appStore == nil {
		return api.ReadinessCheckFailed("postgres store is not configured")
	}
	if err := c.appStore.Ping(ctx); err != nil {
		return api.ReadinessCheckFailed("postgres unavailable")
	}
	return api.ReadinessCheckOK()
}

func (c apiReadinessChecker) checkMigrations(ctx context.Context) api.ReadinessCheck {
	if c.appStore == nil {
		return api.ReadinessCheckFailed("schema migration status unavailable")
	}
	version, err := c.appStore.LatestSchemaMigrationVersion(ctx)
	if err != nil {
		return api.ReadinessCheckFailed("schema migration status unavailable")
	}
	required := c.requiredSchemaMigrationVersion
	if required <= 0 {
		required = requiredSchemaMigrationVersion
	}
	if version < required {
		return api.ReadinessCheckFailed(fmt.Sprintf("schema migration version %d is below required %d", version, required))
	}
	return api.ReadinessCheckOK()
}

func (c apiReadinessChecker) checkTemporal(ctx context.Context) api.ReadinessCheck {
	if c.temporalClient == nil {
		return api.ReadinessCheckFailed("temporal client is not configured")
	}
	if _, err := c.temporalClient.CheckHealth(ctx, &client.CheckHealthRequest{}); err != nil {
		return api.ReadinessCheckFailed("temporal unavailable")
	}
	return api.ReadinessCheckOK()
}

func (c apiReadinessChecker) checkObjectStorage(ctx context.Context) api.ReadinessCheck {
	if strings.ToLower(strings.TrimSpace(c.storageProvider)) != "s3_compatible" {
		return api.ReadinessCheckFailed("object storage provider is unsupported")
	}
	checker, ok := c.objectStore.(interface {
		Check(context.Context) error
	})
	if !ok {
		return api.ReadinessCheckFailed("object storage readiness check is not configured")
	}
	if err := checker.Check(ctx); err != nil {
		return api.ReadinessCheckFailed("object storage unavailable")
	}
	return api.ReadinessCheckOK()
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
		RateLimiter:   api.NewInMemoryAuthRateLimiter(api.AuthRateLimitConfig{}),
		SessionTTL:    sessionTTL,
		CookieSecure:  cfg.AuthCookieSecure,
	}, nil
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
	executor := storedb.NewPgxPoolExecutor(pool)
	return &postgresAppStoreClient{
		recordings: recordings.NewPostgresStore(executor),
		workspaces: workspaces.NewPostgresStore(executor),
		auth:       authstore.NewPostgresStore(executor),
		pool:       pool,
	}, nil
}

type postgresAppStoreClient struct {
	recordings *recordings.PostgresStore
	workspaces *workspaces.PostgresStore
	auth       *authstore.PostgresStore
	pool       *pgxpool.Pool
}

func (s *postgresAppStoreClient) RecordingStore() api.RecordingStore {
	return s.recordings
}

func (s *postgresAppStoreClient) WorkspaceStore() api.WorkspaceStore {
	return s.workspaces
}

func (s *postgresAppStoreClient) AuthStore() appAuthStore {
	return s.auth
}

func (s *postgresAppStoreClient) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *postgresAppStoreClient) LatestSchemaMigrationVersion(ctx context.Context) (int, error) {
	var version int
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(CASE WHEN version ~ '^[0-9]+$' THEN version::integer ELSE NULL END), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema migration version: %w", err)
	}
	return version, nil
}

func (s *postgresAppStoreClient) Close() {
	s.pool.Close()
}
