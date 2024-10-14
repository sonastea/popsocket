package mock_db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sonastea/popsocket/pkg/db"
)

type mockDB struct{}

func New() db.DB {
	return &mockDB{}
}

// Exec wraps the pgxpool.Pool's Exec method.
func (md *mockDB) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil // Exec(ctx, query, args...)
}

// Query wraps the pgxpool.Pool's Query method.
func (md *mockDB) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return nil, nil //  pg.pool.Query(ctx, query, args...)
}

// QueryRow wraps the pgxpool.Pool's QueryRow method.
func (md *mockDB) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return nil // pg.pool.QueryRow(ctx, query, args...)
}

// Ping wraps the pgxpool.Pool's Ping method.
func (md *mockDB) Ping(ctx context.Context) error {
	return nil // pg.pool.Ping(ctx)
}

// Close wraps the pgxpool.Pool's Close method.
func (md *mockDB) Close() {
	// pg.pool.Close()
}
