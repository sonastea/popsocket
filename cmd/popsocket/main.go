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

// Run sets up and starts the PopSocket server.
func Run(ctx context.Context, valkey valkey.Client) error {
	mux := http.NewServeMux()

	db, err := db.NewPostgres(ctx, config.ENV.DATABASE_URL.Value)
	if err != nil {
		return fmt.Errorf("Failed to create postgres instance: %w", err)
	}

	messageStore := popsocket.NewMessageStore(db)
	sessionStore := popsocket.NewSessionStore(db)

	ps, err := popsocket.New(
		valkey,
		popsocket.WithServeMux(mux),
		popsocket.WithAddress(os.Getenv("POPSOCKET_ADDR")),
		popsocket.WithMessageStore(messageStore),
		popsocket.WithSessionStore(sessionStore),
	)
	if err != nil {
		return fmt.Errorf("Failed to create PopSocket: %w", err)
	}

	err = ps.SetupRoutes(mux)
	if err != nil {
		return fmt.Errorf("Failed to setup PopSocket routes: %w", err)
	}

	if err := ps.Start(ctx); err != nil {
		ps.LogError(fmt.Sprintf("Server error: %v", err))
		return err
	}

	return nil
}

func main() {
	ctx := context.Background()

	valkey, err := popsocket.NewValkeyClient()
	if err != nil {
		log.Printf("Failed to created valkey client: %+v", err)
	}

	if err := Run(ctx, valkey); err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}
}
