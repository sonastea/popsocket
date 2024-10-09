package db

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Database interface {
	Close()
	Ping(ctx context.Context) error
	Pool() *pgxpool.Pool
}

type postgres struct {
	db *pgxpool.Pool
}

var conversations []struct {
	ID       string   `json:"id"`
	Messages []string `json:"messages"`
}

var (
	logger slog.Logger
	pg     *postgres
	pgOnce sync.Once
)

func NewPostgres(ctx context.Context, connString string) (Database, error) {
	pgOnce.Do(func() {
		db, err := pgxpool.New(ctx, connString)
		if err != nil {
			log.Fatal(fmt.Errorf("unable to create connection pool: %s", err))
		}
		pg = &postgres{db}
	})

	return pg, nil
}

func (pg *postgres) Pool() *pgxpool.Pool {
	return pg.db
}

func (pg *postgres) Ping(ctx context.Context) error {
	return pg.db.Ping(ctx)
}

func (pg *postgres) Close() {
	pg.db.Close()
}
