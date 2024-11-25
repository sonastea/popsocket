package db

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgres struct {
	pool *pgxpool.Pool
}

var (
	logger slog.Logger
	pg     *postgres
	pgOnce sync.Once
)

// Begin wraps the pgxpool.Pool's Begin method.
func (pg *postgres) Begin(ctx context.Context) (pgx.Tx, error) {
	return pg.pool.Begin(ctx)
}

// BeginTx wraps the pgxpool.Pool's BeginTx method.
func (pg *postgres) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	return pg.pool.BeginTx(ctx, txOptions)
}

// Exec wraps the pgxpool.Pool's Exec method.
func (pg *postgres) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return pg.pool.Exec(ctx, query, args...)
}

// Query wraps the pgxpool.Pool's Query method.
func (pg *postgres) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return pg.pool.Query(ctx, query, args...)
}

// QueryRow wraps the pgxpool.Pool's QueryRow method.
func (pg *postgres) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return pg.pool.QueryRow(ctx, query, args...)
}

// Ping wraps the pgxpool.Pool's Ping method.
func (pg *postgres) Ping(ctx context.Context) error {
	return pg.pool.Ping(ctx)
}

// Close wraps the pgxpool.Pool's Close method.
func (pg *postgres) Close() {
	pg.pool.Close()
}
