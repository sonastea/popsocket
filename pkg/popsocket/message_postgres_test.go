package popsocket

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
)

// Count SQL executions, including transaction control. A deferred Rollback on
// an already closed transaction sends no SQL and is intentionally not counted.
type saveQueryCounts struct {
	operations atomic.Int64
	begins     atomic.Int64
	commits    atomic.Int64
	rollbacks  atomic.Int64
}

func (c *saveQueryCounts) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.operations.Add(1)
	switch data.SQL {
	case "begin":
		c.begins.Add(1)
	case "commit":
		c.commits.Add(1)
	case "rollback":
		c.rollbacks.Add(1)
	}
	return ctx
}

func (*saveQueryCounts) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *saveQueryCounts) reset() {
	c.operations.Store(0)
	c.begins.Store(0)
	c.commits.Store(0)
	c.rollbacks.Store(0)
}

func (c *saveQueryCounts) assertTransactions(t *testing.T, begins, commits, rollbacks int64) {
	t.Helper()
	if got := c.begins.Load(); got != begins {
		t.Errorf("BEGIN count = %d, want %d", got, begins)
	}
	if got := c.commits.Load(); got != commits {
		t.Errorf("COMMIT count = %d, want %d", got, commits)
	}
	if got := c.rollbacks.Load(); got != rollbacks {
		t.Errorf("ROLLBACK count = %d, want %d", got, rollbacks)
	}
}

// Each test gets an isolated schema. The URL must explicitly opt in to real
// PostgreSQL testing; the application's DATABASE_URL is never used here.
func newSavePostgres(tb testing.TB) (*pgxpool.Pool, *saveQueryCounts) {
	tb.Helper()
	url := os.Getenv("POPSOCKET_TEST_DATABASE_URL")
	if url == "" {
		tb.Skip("set POPSOCKET_TEST_DATABASE_URL to run PostgreSQL tests and benchmarks")
	}
	ctx, cancel := context.WithTimeout(tb.Context(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		tb.Fatal(err)
	}
	schema := "save_" + uuid.New().String()
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE"); err != nil {
			tb.Error(err)
		}
	})
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		tb.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		tb.Fatal(err)
	}
	counts := &saveQueryCounts{}
	config.MaxConns = 8
	config.ConnConfig.RuntimeParams["search_path"] = quotedSchema
	config.ConnConfig.RuntimeParams["application_name"] = schema
	config.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	config.ConnConfig.RuntimeParams["default_transaction_isolation"] = "read committed"
	config.ConnConfig.Tracer = counts
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `
		CREATE TABLE "User" (
			id integer PRIMARY KEY, username text NOT NULL, displayname text, photo text
		);
		CREATE TABLE "Conversation" (
			id serial PRIMARY KEY, convid text NOT NULL UNIQUE
		);
		CREATE TABLE "_ConversationToUser" (
			"A" integer NOT NULL REFERENCES "Conversation"(id),
			"B" integer NOT NULL REFERENCES "User"(id),
			UNIQUE ("A", "B")
		);
		CREATE TABLE "Message" (
			id serial PRIMARY KEY,
			"convId" text NOT NULL REFERENCES "Conversation"(convid),
			"recipientId" integer NOT NULL REFERENCES "User"(id),
			"userId" integer NOT NULL REFERENCES "User"(id),
			content text, "createdAt" timestamptz NOT NULL,
			"fromSelf" boolean NOT NULL, read boolean NOT NULL
		);
		INSERT INTO "User" VALUES (1, 'sender', 'Sender', 'sender.png'), (2, 'recipient', NULL, NULL);
	`)
	if err != nil {
		tb.Fatal(err)
	}
	return pool, counts
}

func saveTestMessage(convid string) *ipc.Message {
	content := "hello"
	return &ipc.Message{
		Convid: convid, From: 1, To: 2, Content: &content,
		CreatedAt: "2026-10-01T12:00:00Z",
	}
}

func assertSaveRows(t *testing.T, pool *pgxpool.Pool, conversations, memberships, messages int) {
	t.Helper()
	for _, table := range []struct {
		name string
		want int
	}{
		{"Conversation", conversations},
		{"_ConversationToUser", memberships},
		{"Message", messages},
	} {
		var got int
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table.name}.Sanitize()).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != table.want {
			t.Errorf("%s rows = %d, want %d", table.name, got, table.want)
		}
	}
}

func TestMessageStoreSavePostgres(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, self := range []bool{false, true} {
			t.Run(fmt.Sprintf("existing=%t/self=%t", existing, self), func(t *testing.T) {
				pool, counts := newSavePostgres(t)
				msg := saveTestMessage("conversation")
				if existing {
					// Deliberately leave membership links missing to verify that
					// conflict resolution inserts them using the existing ID.
					if _, err := pool.Exec(t.Context(), `INSERT INTO "Conversation" (id, convid) VALUES (42, $1)`, msg.Convid); err != nil {
						t.Fatal(err)
					}
				}
				members := 2
				if self {
					msg.To, msg.FromSelf = msg.From, true
					members = 1
				}
				store := NewMessageStore(nil, pool)
				for i := range 2 {
					counts.reset()
					saved, err := store.Save(t.Context(), msg)
					if err != nil {
						t.Fatal(err)
					}
					counts.assertTransactions(t, 1, 1, 0)
					if saved != msg {
						t.Fatal("Save did not return the saved message")
					}
					if !self && (saved.GetFromUser() != "Sender" || saved.GetFromPhoto() != "sender.png") {
						t.Fatalf("sender metadata = %q, %q", saved.GetFromUser(), saved.GetFromPhoto())
					}
					assertSaveRows(t, pool, 1, members, i+1)
				}
				if existing {
					var linked int
					if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM "_ConversationToUser" WHERE "A" = 42`).Scan(&linked); err != nil {
						t.Fatal(err)
					}
					if linked != members {
						t.Errorf("members linked to actual ID 42 = %d, want %d", linked, members)
					}
				}
			})
		}
	}
}

func TestMessageStoreSavePostgresConcurrent(t *testing.T) {
	pool, counts := newSavePostgres(t)
	store := NewMessageStore(nil, pool)
	const writers = 16
	start := make(chan struct{})
	errors := make(chan error, writers)
	var wg sync.WaitGroup
	counts.reset()
	for range writers {
		wg.Go(func() {
			<-start
			_, err := store.Save(t.Context(), saveTestMessage("shared"))
			errors <- err
		})
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
	counts.assertTransactions(t, writers, writers, 0)
	assertSaveRows(t, pool, 1, 2, writers)
}

func TestMessageStoreSavePostgresUncommittedConflict(t *testing.T) {
	t.Run("creator_commits", func(t *testing.T) { testSaveUncommittedConflict(t, true) })
	t.Run("creator_rolls_back", func(t *testing.T) { testSaveUncommittedConflict(t, false) })
}

func testSaveUncommittedConflict(t *testing.T, creatorCommits bool) {
	t.Helper()
	pool, counts := newSavePostgres(t)
	config := pool.Config().ConnConfig.Copy()
	config.Tracer = nil
	creator, err := pgx.ConnectConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer creator.Close(context.Background())
	tx, err := creator.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `INSERT INTO "Conversation" (id, convid) VALUES (42, 'shared')`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	counts.reset()
	result := make(chan error, 1)
	go func() {
		_, err := NewMessageStore(nil, pool).Save(ctx, saveTestMessage("shared"))
		result <- err
	}()
	// Observe PostgreSQL's lock wait before ending the competing transaction.
	// On commit, the fallback SELECT must see a row absent from INSERT's snapshot.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity
			WHERE application_name = $1 AND wait_event_type = 'Lock'
			AND query LIKE 'INSERT INTO "Conversation"%'
		)`, config.RuntimeParams["application_name"]).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("Save returned before the conflicting transaction ended: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if creatorCommits {
		err = tx.Commit(ctx)
	} else {
		err = tx.Rollback(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	counts.assertTransactions(t, 1, 1, 0)
	assertSaveRows(t, pool, 1, 2, 1)
}

func TestMessageStoreSavePostgresRollback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    string
		code     string
		existing bool
		commit   bool
	}{
		{
			name:  "conversation",
			setup: `ALTER TABLE "Conversation" ADD CHECK (convid <> 'conversation')`,
			code:  "23514",
		},
		{name: "membership", code: "23503"},
		{name: "existing_membership", code: "23503", existing: true},
		{
			name:  "message",
			setup: `ALTER TABLE "Message" ADD CHECK (content <> 'hello')`,
			code:  "23514",
		},
		{
			name: "sender",
			// FK references follow the renamed table. Only the sender lookup
			// uses this failing view, after membership and message insertion.
			setup: `CREATE FUNCTION fail_sender() RETURNS boolean LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'sender lookup failed' USING ERRCODE = 'P0001'; END;
				$$;
				ALTER TABLE "User" RENAME TO user_data;
				CREATE VIEW "User" AS SELECT * FROM user_data WHERE fail_sender()`,
			code: "P0001",
		},
		{
			name: "commit",
			setup: `CREATE TABLE allowed_content (content text PRIMARY KEY);
				ALTER TABLE "Message" ADD FOREIGN KEY (content) REFERENCES allowed_content(content)
				DEFERRABLE INITIALLY DEFERRED`,
			code: "23503", commit: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, counts := newSavePostgres(t)
			msg := saveTestMessage("conversation")
			if tc.setup != "" {
				if _, err := pool.Exec(t.Context(), tc.setup); err != nil {
					t.Fatal(err)
				}
			} else {
				msg.To = 999 // A real FK failure must not restart the transaction.
			}
			conversations := 0
			if tc.existing {
				if _, err := pool.Exec(t.Context(), `INSERT INTO "Conversation" (id, convid) VALUES (42, $1)`, msg.Convid); err != nil {
					t.Fatal(err)
				}
				conversations = 1
			}
			counts.reset()
			saved, err := NewMessageStore(nil, pool).Save(t.Context(), msg)
			var pgErr *pgconn.PgError
			if saved != nil || !errors.As(err, &pgErr) || pgErr.Code != tc.code {
				t.Fatalf("Save = %v, %v; want PostgreSQL error %s", saved, err, tc.code)
			}
			if tc.commit {
				// PostgreSQL itself rolls back a failed deferred constraint at COMMIT.
				counts.assertTransactions(t, 1, 1, 0)
			} else {
				counts.assertTransactions(t, 1, 0, 1)
			}
			assertSaveRows(t, pool, conversations, 0, 0)
		})
	}
}

func TestMessageStoreSavePostgresSenderFallback(t *testing.T) {
	pool, counts := newSavePostgres(t)
	msg := saveTestMessage("conversation")
	msg.From, msg.To = 2, 1
	counts.reset()
	saved, err := NewMessageStore(nil, pool).Save(t.Context(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetFromUser() != "recipient" || saved.FromPhoto != nil {
		t.Fatalf("sender metadata = %q, %v", saved.GetFromUser(), saved.FromPhoto)
	}
	counts.assertTransactions(t, 1, 1, 0)
}
