package popsocket

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	mock_db "github.com/sonastea/popsocket/internal/mock/db"
)

type saveFailureTx struct {
	pgx.Tx
	queryRow func(...any) error
	batchErr error
	commit   func() error
	rollback func() error
}

func (tx *saveFailureTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return mock_db.RowFunc(tx.queryRow)
}

func (tx *saveFailureTx) SendBatch(_ context.Context, batch *pgx.Batch) pgx.BatchResults {
	return &saveFailureBatch{tx: tx, batch: batch}
}

func (tx *saveFailureTx) Commit(context.Context) error { return tx.commit() }

func (tx *saveFailureTx) Rollback(context.Context) error { return tx.rollback() }

type saveFailureBatch struct {
	pgx.BatchResults
	tx    *saveFailureTx
	batch *pgx.Batch
}

func (b *saveFailureBatch) QueryRow() pgx.Row { return mock_db.RowFunc(b.tx.queryRow) }

func (b *saveFailureBatch) Close() error {
	if b.tx.batchErr != nil {
		return b.tx.batchErr
	}
	for _, query := range b.batch.QueuedQueries {
		if query.Fn != nil {
			if err := query.Fn(b); err != nil {
				return err
			}
		}
	}
	return nil
}

// Inject errors that are difficult to induce with ordinary table constraints.
// Real constraint failures and transaction atomicity are covered in PostgreSQL.
func TestMessageStoreSaveErrors(t *testing.T) {
	wantErr := errors.New("database failure")
	for _, stage := range []string{"begin", "insert", "resolve", "resolve_missing", "batch", "sender", "sender_missing", "commit"} {
		t.Run(stage, func(t *testing.T) {
			database := mock_db.New(t)
			begins, commits, rollbacks, queries := 0, 0, 0, 0
			tx := &saveFailureTx{
				queryRow: func(dest ...any) error {
					queries++
					if stage == "insert" || stage == "sender" && queries == 2 {
						return wantErr
					}
					if stage == "sender_missing" && queries == 2 {
						return pgx.ErrNoRows
					}
					if stage == "resolve" || stage == "resolve_missing" {
						if queries == 1 || stage == "resolve_missing" {
							return pgx.ErrNoRows
						}
						return wantErr
					}
					if queries == 1 {
						*dest[0].(*int) = 42
					}
					return nil
				},
				commit: func() error {
					commits++
					return wantErr
				},
				rollback: func() error {
					rollbacks++
					return nil
				},
			}
			if stage == "batch" {
				tx.batchErr = wantErr
			}
			database.BeginFunc = func(context.Context) (pgx.Tx, error) {
				begins++
				if stage == "begin" {
					return nil, wantErr
				}
				return tx, nil
			}
			saved, err := NewMessageStore(nil, database).Save(t.Context(), saveTestMessage("conversation"))
			want := wantErr
			if stage == "resolve_missing" || stage == "sender_missing" {
				want = pgx.ErrNoRows
			}
			if saved != nil || !errors.Is(err, want) {
				t.Fatalf("Save = %v, %v; want nil, %v", saved, err, want)
			}
			wantCommits, wantRollbacks := 0, 1
			if stage == "commit" {
				wantCommits = 1
			}
			if stage == "begin" {
				wantRollbacks = 0
			}
			if begins != 1 || commits != wantCommits || rollbacks != wantRollbacks {
				t.Fatalf("begin/commit/rollback calls = %d/%d/%d, want 1/%d/%d", begins, commits, rollbacks, wantCommits, wantRollbacks)
			}
		})
	}
}

func TestMessageStoreSaveWrappedNoRows(t *testing.T) {
	for _, noRows := range []error{pgx.ErrNoRows, sql.ErrNoRows} {
		t.Run(noRows.Error(), func(t *testing.T) {
			database := mock_db.New(t)
			queries, rollbacks := 0, 0
			tx := &saveFailureTx{
				queryRow: func(dest ...any) error {
					queries++
					if queries == 1 {
						return fmt.Errorf("conflict: %w", noRows)
					}
					*dest[0].(*int) = 42
					return nil
				},
				commit:   func() error { return nil },
				rollback: func() error { rollbacks++; return nil },
			}
			database.BeginFunc = func(context.Context) (pgx.Tx, error) { return tx, nil }
			msg := saveTestMessage("self")
			msg.To, msg.FromSelf = msg.From, true
			if _, err := NewMessageStore(nil, database).Save(t.Context(), msg); err != nil {
				t.Fatal(err)
			}
			if queries != 2 || rollbacks != 1 {
				t.Fatalf("query/rollback calls = %d/%d, want 2/1", queries, rollbacks)
			}
		})
	}
}
