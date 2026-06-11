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
