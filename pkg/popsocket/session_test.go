package popsocket

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	mock_db "github.com/sonastea/popsocket/internal/mock/db"
)

func TestSessionStoreMissingSession(t *testing.T) {
	ctx := t.Context()
	database := mock_db.New(t)
	database.QueryRowFunc = func(context.Context, string, ...any) pgx.Row {
		return mock_db.RowFunc(func(...any) error { return pgx.ErrNoRows })
	}
	store := NewSessionStore(database)

	_, err := store.Find(ctx, "missing")
	if err == nil || err.Error() != SESSION_UNAUTHORIZED {
		t.Fatalf("Expected unauthorized session, got %v", err)
	}
	expired, err := store.HasExpired(ctx, "missing")
	if err != nil || !expired {
		t.Fatalf("Expected missing session to be expired, got expired=%v, err=%v", expired, err)
	}
}
