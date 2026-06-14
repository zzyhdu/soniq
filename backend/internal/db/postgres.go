package db

import "context"

// PostgresRow is the subset of database row behavior used by Postgres stores.
type PostgresRow interface {
	Scan(dest ...any) error
}

// PostgresRows is the subset of database rows behavior used by Postgres stores.
type PostgresRows interface {
	Close()
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// PostgresExecutor is the subset of database behavior used by Postgres stores.
type PostgresExecutor interface {
	QueryRow(ctx context.Context, query string, args ...any) PostgresRow
	Query(ctx context.Context, query string, args ...any) (PostgresRows, error)
}

// PostgresTx is the subset of transaction behavior used by stores that need
// several database mutations to commit or roll back together.
type PostgresTx interface {
	PostgresExecutor
	Exec(ctx context.Context, query string, args ...any) error
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// PostgresTransactor starts Postgres transactions.
type PostgresTransactor interface {
	Begin(ctx context.Context) (PostgresTx, error)
}
