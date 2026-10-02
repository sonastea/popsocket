package mock_db

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sonastea/popsocket/pkg/db"
)

var _ db.DB = (*mockDB)(nil)

type mockDB struct {
	t testing.TB

	BeginFunc    func(ctx context.Context) (pgx.Tx, error)
	BeginTxFunc  func(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	ExecFunc     func(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
	QueryFunc    func(ctx context.Context, query string, args ...any) (pgx.Rows, error)
	QueryRowFunc func(ctx context.Context, query string, args ...any) pgx.Row
	PingFunc     func(ctx context.Context) error
	CloseFunc    func()
}

// New creates a mock whose behavior is configured by the test through function fields.
// Calling an unconfigured method fails the test, even if its error is ignored.
func New(t testing.TB) *mockDB {
	return &mockDB{t: t}
}

func (md *mockDB) unexpectedCall(method string) error {
	md.t.Helper()
	err := fmt.Errorf("mock_db: unexpected call to %s", method)
	md.t.Error(err)
	return err
}

func (md *mockDB) Begin(ctx context.Context) (pgx.Tx, error) {
	md.t.Helper()
	if md.BeginFunc == nil {
		return nil, md.unexpectedCall("Begin")
	}
	return md.BeginFunc(ctx)
}

func (md *mockDB) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	md.t.Helper()
	if md.BeginTxFunc == nil {
		return nil, md.unexpectedCall("BeginTx")
	}
	return md.BeginTxFunc(ctx, txOptions)
}

func (md *mockDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	md.t.Helper()
	if md.ExecFunc == nil {
		return pgconn.CommandTag{}, md.unexpectedCall("Exec")
	}
	return md.ExecFunc(ctx, query, args...)
}

func (md *mockDB) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	md.t.Helper()
	if md.QueryFunc == nil {
		return nil, md.unexpectedCall("Query")
	}
	return md.QueryFunc(ctx, query, args...)
}

func (md *mockDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	md.t.Helper()
	if md.QueryRowFunc == nil {
		err := md.unexpectedCall("QueryRow")
		return RowFunc(func(...any) error { return err })
	}
	return md.QueryRowFunc(ctx, query, args...)
}

func (md *mockDB) Ping(ctx context.Context) error {
	md.t.Helper()
	if md.PingFunc == nil {
		return md.unexpectedCall("Ping")
	}
	return md.PingFunc(ctx)
}

func (md *mockDB) Close() {
	md.t.Helper()
	if md.CloseFunc == nil {
		_ = md.unexpectedCall("Close")
		return
	}
	md.CloseFunc()
}

// RowFunc lets a test provide the Scan behavior of a pgx.Row.
type RowFunc func(dest ...any) error

func (scan RowFunc) Scan(dest ...any) error {
	return scan(dest...)
}
