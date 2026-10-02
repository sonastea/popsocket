package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/sonastea/popsocket/pkg/config"
	"github.com/sonastea/popsocket/pkg/db"
	"github.com/sonastea/popsocket/pkg/popsocket"
	"github.com/valkey-io/valkey-go"
)

type runOption func(*runConfig)

type runConfig struct {
	db db.DB
}

// WithDB overrides the database used by Run, useful in tests.
func WithDB(override db.DB) runOption {
	return func(rc *runConfig) {
		if override != nil {
			rc.db = override
		}
	}
}

// Run sets up and starts the PopSocket server.
func Run(ctx context.Context, valkey valkey.Client, opts ...runOption) error {
	if err := config.LoadEnvVars(); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	mux := http.NewServeMux()

	rc := &runConfig{}
	for _, opt := range opts {
		opt(rc)
	}

	if rc.db == nil {
		d, err := db.NewPostgres(ctx, config.ENV.DATABASE_URL.Value)
		if err != nil {
			return fmt.Errorf("failed to create database instance: %w", err)
		}
		rc.db = d
		defer d.Close()
	}
	database := rc.db

	if err := database.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := validateDatabaseSchema(ctx, database); err != nil {
		return err
	}

	messageStore := popsocket.NewMessageStore(valkey, database)
	sessionStore := popsocket.NewSessionStore(database)

	messageService := popsocket.NewMessageService(messageStore)
	sessionMiddleware := popsocket.NewSessionMiddleware(sessionStore)

	ps, err := popsocket.New(
		valkey,
		popsocket.WithServeMux(mux),
		popsocket.WithAddress(os.Getenv("POPSOCKET_ADDR")),
		popsocket.WithMessageService(messageService),
		popsocket.WithSessionMiddleware(sessionMiddleware),
	)
	if err != nil {
		return fmt.Errorf("failed to create PopSocket: %w", err)
	}

	err = ps.SetupRoutes(mux)
	if err != nil {
		return fmt.Errorf("failed to setup PopSocket routes: %w", err)
	}

	if err := ps.Start(ctx); err != nil {
		ps.LogError(fmt.Sprintf("Server error: %v", err))
		return err
	}

	return nil
}

// validateDatabaseSchema checks the columns used by the bundled stores.
// kpoppop owns the schema and migrations; startup only verifies compatibility.
func validateDatabaseSchema(ctx context.Context, database db.DB) error {
	for _, query := range []string{
		`SELECT s.sid, s.data, s."expiresAt" FROM "Session" s WHERE 1 = 0`,
		`SELECT u.id, u.username, u.displayname, u.photo, u.status FROM "User" u WHERE 1 = 0`,
		`SELECT u.id, u."discordId" FROM "DiscordUser" u WHERE 1 = 0`,
		`SELECT c.id, c.convid FROM "Conversation" c WHERE 1 = 0`,
		`SELECT cu."A", cu."B" FROM "_ConversationToUser" cu WHERE 1 = 0`,
		`SELECT m."convId", m."recipientId", m."userId", m.content, m."createdAt", m."fromSelf", m.read FROM "Message" m WHERE 1 = 0`,
	} {
		if _, err := database.Exec(ctx, query); err != nil {
			return fmt.Errorf("database schema is unavailable or incompatible; set DATABASE_URL to the database migrated by kpoppop: %w", err)
		}
	}
	return nil
}

func main() {
	ctx := context.Background()

	valkey, err := popsocket.NewValkeyClient()
	if err != nil {
		log.Fatalf("Failed to create valkey client: %v", err)
	}

	if err := Run(ctx, valkey); err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}
}
