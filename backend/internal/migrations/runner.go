package migrations

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	storedb "github.com/zzyhdu/soniq/backend/internal/db"
)

var upMigrationFilePattern = regexp.MustCompile(`^([0-9]+)_.+\.up\.sql$`)

const createSchemaMigrationsTableSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL
)`

// Migration is one application database migration.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Result summarizes one migration run.
type Result struct {
	Applied         []Migration
	Skipped         []Migration
	CurrentVersion  int
	RequiredVersion int
}

type database interface {
	storedb.PostgresExecutor
	storedb.PostgresTransactor
}

// Runner applies application migrations to Soniq Postgres.
type Runner struct {
	db         database
	migrations []Migration
}

// NewRunner creates a migration runner.
func NewRunner(db database, migrations []Migration) Runner {
	return Runner{
		db:         db,
		migrations: append([]Migration(nil), migrations...),
	}
}

// LoadUpMigrations reads and sorts up migrations from an fs.FS.
func LoadUpMigrations(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	migrations := []Migration{}
	seenVersions := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		matches := upMigrationFilePattern.FindStringSubmatch(name)
		if len(matches) != 2 {
			continue
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", name, err)
		}
		if existing := seenVersions[version]; existing != "" {
			return nil, fmt.Errorf("duplicate migration version %d: %s and %s", version, existing, name)
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		sql := strings.TrimSpace(string(body))
		if sql == "" {
			return nil, fmt.Errorf("migration %s is empty", name)
		}
		seenVersions[version] = name
		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     sql,
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no up migrations found")
	}
	return migrations, nil
}

// RequiredVersion returns the highest migration version in the set.
func RequiredVersion(migrations []Migration) int {
	required := 0
	for _, migration := range migrations {
		if migration.Version > required {
			required = migration.Version
		}
	}
	return required
}

// Apply applies all pending migrations in order.
func (r Runner) Apply(ctx context.Context) (Result, error) {
	if r.db == nil {
		return Result{}, fmt.Errorf("migration database is required")
	}
	if len(r.migrations) == 0 {
		return Result{}, fmt.Errorf("migrations are required")
	}
	migrations := append([]Migration(nil), r.migrations...)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	if err := r.execInTransaction(ctx, createSchemaMigrationsTableSQL); err != nil {
		return Result{}, fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	result := Result{RequiredVersion: RequiredVersion(migrations)}
	for _, migration := range migrations {
		applied, err := r.migrationApplied(ctx, migration.Version)
		if err != nil {
			return Result{}, err
		}
		if applied {
			result.Skipped = append(result.Skipped, migration)
			if migration.Version > result.CurrentVersion {
				result.CurrentVersion = migration.Version
			}
			continue
		}
		if err := r.applyMigration(ctx, migration); err != nil {
			return Result{}, err
		}
		result.Applied = append(result.Applied, migration)
		if migration.Version > result.CurrentVersion {
			result.CurrentVersion = migration.Version
		}
	}
	return result, nil
}

func (r Runner) migrationApplied(ctx context.Context, version int) (bool, error) {
	var applied bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, strconv.Itoa(version)).Scan(&applied); err != nil {
		return false, fmt.Errorf("read schema migration version %d: %w", version, err)
	}
	return applied, nil
}

func (r Runner) applyMigration(ctx context.Context, migration Migration) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := tx.Exec(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.Name, err)
	}
	if err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, NOW()) ON CONFLICT (version) DO NOTHING`, strconv.Itoa(migration.Version)); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.Name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.Name, err)
	}
	committed = true
	return nil
}

func (r Runner) execInTransaction(ctx context.Context, query string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := tx.Exec(ctx, query); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}
