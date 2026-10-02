package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	mock_db "github.com/sonastea/popsocket/internal/mock/db"
	"github.com/sonastea/popsocket/pkg/popsocket"
	"github.com/sonastea/popsocket/pkg/testutil"
	"github.com/valkey-io/valkey-go"
)

// TestRun uses a mock database to exercise the missing-session path.
func TestRun(t *testing.T) {
	database := mock_db.New(t)
	database.PingFunc = func(context.Context) error { return nil }
	database.ExecFunc = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, nil
	}
	var sessionQueried atomic.Bool
	database.QueryRowFunc = func(context.Context, string, ...any) pgx.Row {
		sessionQueried.Store(true)
		return mock_db.RowFunc(func(...any) error { return pgx.ErrNoRows })
	}

	ctx, wsURL, cookie := startRun(t, WithDB(database))
	assertUnauthorized(t, ctx, wsURL, cookie)
	if !sessionQueried.Load() {
		t.Fatal("Expected a session lookup before rejecting the request")
	}
}

func TestRunMissingConfiguration(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "SESSION_SECRET_KEY"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgresql://localhost:5432/popsocket_test")
			t.Setenv("SESSION_SECRET_KEY", "test-secret")
			t.Setenv(key, "")
			err := Run(t.Context(), nil)
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Expected configuration error for %s, got %v", key, err)
			}
		})
	}
}

func TestRunInvalidDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "invalid-conn-string")
	t.Setenv("SESSION_SECRET_KEY", "test-secret")
	err := Run(t.Context(), nil)
	if err == nil || !strings.Contains(err.Error(), "failed to create database instance") {
		t.Fatalf("Expected invalid database URL error, got %v", err)
	}
}

func TestRunDatabaseErrors(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://localhost:5432/popsocket_test")
	t.Setenv("SESSION_SECRET_KEY", "test-secret")
	connectionErr := errors.New("database unavailable")
	schemaErr := &pgconn.PgError{Code: "42P01", Message: `relation "Session" does not exist`}
	for _, tt := range []struct {
		ping        func(context.Context) error
		exec        func(context.Context, string, ...any) (pgconn.CommandTag, error)
		wantErr     error
		name        string
		wantMessage string
	}{
		{
			name:        "connection error",
			ping:        func(context.Context) error { return connectionErr },
			wantErr:     connectionErr,
			wantMessage: "failed to connect to database",
		},
		{
			name: "missing schema",
			ping: func(context.Context) error { return nil },
			exec: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, schemaErr
			},
			wantErr:     schemaErr,
			wantMessage: "database migrated by kpoppop",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			database := mock_db.New(t)
			database.PingFunc = tt.ping
			database.ExecFunc = tt.exec
			err := Run(t.Context(), nil, WithDB(database))
			if !errors.Is(err, tt.wantErr) || !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("Expected %s wrapping %v, got %v", tt.wantMessage, tt.wantErr, err)
			}
		})
	}
}

func startRun(t *testing.T, opts ...runOption) (context.Context, string, string) {
	t.Helper()
	s := miniredis.RunT(t)
	t.Setenv("REDIS_URL", s.Addr())
	t.Setenv("DATABASE_URL", "postgresql://localhost:5432/popsocket_test")
	t.Setenv("SESSION_SECRET_KEY", "test-secret")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	t.Setenv("POPSOCKET_ADDR", addr)

	cookie, err := testutil.CreateSignedCookie("foo", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	client, err := popsocket.NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		errCh <- Run(ctx, client, opts...)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && err != http.ErrServerClosed {
				t.Errorf("Server encountered an error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("Timed out waiting for server shutdown")
		}
	})

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return ctx, "ws://" + addr + "/", cookie
		}
		select {
		case err := <-errCh:
			t.Fatalf("Server stopped before becoming ready: %v", err)
		case <-ctx.Done():
			t.Fatalf("Server did not start: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func assertUnauthorized(t *testing.T, ctx context.Context, wsURL, cookie string) {
	t.Helper()
	header := http.Header{}
	header.Add("Cookie", fmt.Sprintf("connect.sid=%s", cookie))
	conn, response, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err == nil {
		conn.CloseNow()
		t.Fatal("Expected unauthorized response, got a websocket connection")
	}
	if response == nil {
		t.Fatalf("Expected an HTTP response, got connection error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401, got %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != popsocket.SESSION_UNAUTHORIZED {
		t.Fatalf("Expected body %q, got %q", popsocket.SESSION_UNAUTHORIZED, body)
	}
}
