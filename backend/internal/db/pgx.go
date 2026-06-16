package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxPoolExecutor adapts pgxpool.Pool to the small Postgres interfaces used by stores.
type PgxPoolExecutor struct {
	pool                                 *pgxpool.Pool
	simpleProtocolForUnparameterizedExec bool
}

var (
	_ PostgresExecutor   = (*PgxPoolExecutor)(nil)
	_ PostgresTransactor = (*PgxPoolExecutor)(nil)
	_ PostgresTx         = pgxTx{}
)

// PgxPoolExecutorOption configures a PgxPoolExecutor.
type PgxPoolExecutorOption func(*PgxPoolExecutor)

// WithSimpleProtocolForUnparameterizedExec executes transaction Exec calls
// without arguments using pgx simple protocol.
func WithSimpleProtocolForUnparameterizedExec() PgxPoolExecutorOption {
	return func(e *PgxPoolExecutor) {
		e.simpleProtocolForUnparameterizedExec = true
	}
}

// NewPgxPoolExecutor creates a Postgres executor backed by a pgx connection pool.
func NewPgxPoolExecutor(pool *pgxpool.Pool, options ...PgxPoolExecutorOption) *PgxPoolExecutor {
	executor := &PgxPoolExecutor{pool: pool}
	for _, option := range options {
		option(executor)
	}
	return executor
}

func (e *PgxPoolExecutor) QueryRow(ctx context.Context, query string, args ...any) PostgresRow {
	return e.pool.QueryRow(ctx, query, args...)
}

func (e *PgxPoolExecutor) Query(ctx context.Context, query string, args ...any) (PostgresRows, error) {
	return e.pool.Query(ctx, query, args...)
}

func (e *PgxPoolExecutor) Begin(ctx context.Context) (PostgresTx, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return pgxTx{
		tx:                                   tx,
		simpleProtocolForUnparameterizedExec: e.simpleProtocolForUnparameterizedExec,
	}, nil
}

type pgxTx struct {
	tx                                   pgx.Tx
	simpleProtocolForUnparameterizedExec bool
}

func (tx pgxTx) QueryRow(ctx context.Context, query string, args ...any) PostgresRow {
	return tx.tx.QueryRow(ctx, query, args...)
}

func (tx pgxTx) Query(ctx context.Context, query string, args ...any) (PostgresRows, error) {
	return tx.tx.Query(ctx, query, args...)
}

func (tx pgxTx) Exec(ctx context.Context, query string, args ...any) error {
	if tx.simpleProtocolForUnparameterizedExec && len(args) == 0 {
		_, err := tx.tx.Exec(ctx, query, pgx.QueryExecModeSimpleProtocol)
		return err
	}
	_, err := tx.tx.Exec(ctx, query, args...)
	return err
}

func (tx pgxTx) Commit(ctx context.Context) error {
	return tx.tx.Commit(ctx)
}

func (tx pgxTx) Rollback(ctx context.Context) error {
	return tx.tx.Rollback(ctx)
}
