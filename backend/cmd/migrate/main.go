package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zzyhdu/soniq/backend/internal/config"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	appmigrations "github.com/zzyhdu/soniq/backend/internal/migrations"
	"github.com/zzyhdu/soniq/backend/internal/observability"
	"github.com/zzyhdu/soniq/backend/internal/version"
	sqlmigrations "github.com/zzyhdu/soniq/backend/migrations"
)

func main() {
	showVersion := flag.Bool("version", false, "print build version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Summary("soniq-migrate"))
		return
	}

	cfg := config.LoadFromEnv()
	logger, err := observability.NewLogger(observability.LoggerConfig{
		Service: "soniq-migrate",
		Format:  cfg.LogFormat,
		Level:   cfg.LogLevel,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure logger: %v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	logger.Info("starting soniq-migrate",
		slog.String("event", "migration_starting"),
		slog.String("version", version.Version),
		slog.String("commit", version.Commit),
	)
	if err := run(context.Background(), cfg, logger); err != nil {
		logger.Error("migration failed", slog.String("event", "migration_failed"), slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		return fmt.Errorf("POSTGRES_DSN is required")
	}
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	migrations, err := appmigrations.LoadUpMigrations(sqlmigrations.UpFiles)
	if err != nil {
		return err
	}
	executor := storedb.NewPgxPoolExecutor(pool, storedb.WithSimpleProtocolForUnparameterizedExec())
	result, err := appmigrations.NewRunner(executor, migrations).Apply(ctx)
	if err != nil {
		return err
	}
	for _, migration := range result.Applied {
		logger.Info("applied migration",
			slog.String("event", "migration_applied"),
			slog.Int("migration_version", migration.Version),
			slog.String("migration_name", migration.Name),
		)
	}
	logger.Info("application migrations are up to date",
		slog.String("event", "migration_completed"),
		slog.Int("applied_count", len(result.Applied)),
		slog.Int("skipped_count", len(result.Skipped)),
		slog.Int("current_version", result.CurrentVersion),
		slog.Int("required_version", result.RequiredVersion),
	)
	return nil
}
