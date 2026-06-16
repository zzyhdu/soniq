package migrations

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	storedb "github.com/zzyhdu/soniq/backend/internal/db"
)

func TestLoadUpMigrationsSortsByVersion(t *testing.T) {
	migrations, err := LoadUpMigrations(fstest.MapFS{
		"0002_second.up.sql":  {Data: []byte("ALTER TABLE example ADD COLUMN name TEXT;")},
		"0001_first.up.sql":   {Data: []byte("CREATE TABLE example (id TEXT PRIMARY KEY);")},
		"0001_first.down.sql": {Data: []byte("DROP TABLE example;")},
	})
	if err != nil {
		t.Fatalf("LoadUpMigrations returned error: %v", err)
	}

	if got, want := len(migrations), 2; got != want {
		t.Fatalf("migrations = %d, want %d", got, want)
	}
	if migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("migration versions = %d/%d, want 1/2", migrations[0].Version, migrations[1].Version)
	}
	if RequiredVersion(migrations) != 2 {
		t.Fatalf("RequiredVersion = %d, want 2", RequiredVersion(migrations))
	}
}

func TestLoadUpMigrationsRejectsDuplicateVersions(t *testing.T) {
	_, err := LoadUpMigrations(fstest.MapFS{
		"0001_first.up.sql": {Data: []byte("SELECT 1;")},
		"0001_again.up.sql": {Data: []byte("SELECT 2;")},
	})
	if err == nil {
		t.Fatal("LoadUpMigrations returned nil error, want duplicate version error")
	}
}

func TestLoadUpMigrationsRejectsEmptySet(t *testing.T) {
	_, err := LoadUpMigrations(fstest.MapFS{
		"README.md": {Data: []byte("ignore")},
	})
	if err == nil {
		t.Fatal("LoadUpMigrations returned nil error, want empty migration set error")
	}
	if !errors.Is(err, fs.ErrNotExist) && !strings.Contains(err.Error(), "no up migrations") {
		t.Fatalf("error = %v, want no up migrations", err)
	}
}

func TestRunnerAppliesOnlyPendingMigrations(t *testing.T) {
	db := newMigrationDBSpy(map[string]bool{"1": true})
	runner := NewRunner(db, []Migration{
		{Version: 1, Name: "0001_first.up.sql", SQL: "CREATE TABLE first (id TEXT PRIMARY KEY);"},
		{Version: 2, Name: "0002_second.up.sql", SQL: "ALTER TABLE first ADD COLUMN name TEXT;"},
	})

	result, err := runner.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if got, want := len(result.Applied), 1; got != want {
		t.Fatalf("applied migrations = %d, want %d", got, want)
	}
	if result.Applied[0].Version != 2 {
		t.Fatalf("applied version = %d, want 2", result.Applied[0].Version)
	}
	if got, want := len(result.Skipped), 1; got != want {
		t.Fatalf("skipped migrations = %d, want %d", got, want)
	}
	if !db.applied["2"] {
		t.Fatal("version 2 was not recorded as applied")
	}
	if db.commits != 2 {
		t.Fatalf("commits = %d, want 2", db.commits)
	}
	if db.rollbacks != 0 {
		t.Fatalf("rollbacks = %d, want 0", db.rollbacks)
	}
	if !queriesContain(db.execs, "ALTER TABLE first ADD COLUMN name TEXT") {
		t.Fatalf("executed queries = %+v, want migration SQL", db.execs)
	}
}

func TestRunnerRollsBackFailedMigration(t *testing.T) {
	db := newMigrationDBSpy(nil)
	db.execErr = errors.New("apply failed")
	db.execErrContains = "BROKEN"
	runner := NewRunner(db, []Migration{
		{Version: 1, Name: "0001_first.up.sql", SQL: "BROKEN;"},
	})

	_, err := runner.Apply(context.Background())
	if err == nil {
		t.Fatal("Apply returned nil error, want migration failure")
	}
	if db.applied["1"] {
		t.Fatal("failed version was recorded as applied")
	}
	if db.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want 1", db.rollbacks)
	}
}

type migrationDBSpy struct {
	applied         map[string]bool
	execs           []string
	execErr         error
	execErrContains string
	commits         int
	rollbacks       int
}

func newMigrationDBSpy(applied map[string]bool) *migrationDBSpy {
	copied := map[string]bool{}
	for version, ok := range applied {
		copied[version] = ok
	}
	return &migrationDBSpy{applied: copied}
}

func (db *migrationDBSpy) QueryRow(_ context.Context, query string, args ...any) storedb.PostgresRow {
	if strings.Contains(query, "SELECT EXISTS") {
		version, _ := args[0].(string)
		return migrationRow{values: []any{db.applied[version]}}
	}
	return migrationRow{err: sql.ErrNoRows}
}

func (db *migrationDBSpy) Query(context.Context, string, ...any) (storedb.PostgresRows, error) {
	return nil, errors.New("unexpected query")
}

func (db *migrationDBSpy) Begin(context.Context) (storedb.PostgresTx, error) {
	return &migrationTxSpy{db: db}, nil
}

type migrationTxSpy struct {
	db *migrationDBSpy
}

func (tx *migrationTxSpy) QueryRow(ctx context.Context, query string, args ...any) storedb.PostgresRow {
	return tx.db.QueryRow(ctx, query, args...)
}

func (tx *migrationTxSpy) Query(ctx context.Context, query string, args ...any) (storedb.PostgresRows, error) {
	return tx.db.Query(ctx, query, args...)
}

func (tx *migrationTxSpy) Exec(_ context.Context, query string, args ...any) error {
	tx.db.execs = append(tx.db.execs, query)
	if tx.db.execErr != nil && (tx.db.execErrContains == "" || strings.Contains(query, tx.db.execErrContains)) {
		return tx.db.execErr
	}
	if strings.Contains(query, "INSERT INTO schema_migrations") {
		version, _ := args[0].(string)
		tx.db.applied[version] = true
	}
	return nil
}

func (tx *migrationTxSpy) Commit(context.Context) error {
	tx.db.commits++
	return nil
}

func (tx *migrationTxSpy) Rollback(context.Context) error {
	tx.db.rollbacks++
	return nil
}

type migrationRow struct {
	values []any
	err    error
}

func (r migrationRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return sql.ErrNoRows
	}
	for index, value := range r.values {
		switch target := dest[index].(type) {
		case *bool:
			*target = value.(bool)
		default:
			return sql.ErrNoRows
		}
	}
	return nil
}

func queriesContain(queries []string, want string) bool {
	for _, query := range queries {
		if strings.Contains(query, want) {
			return true
		}
	}
	return false
}
