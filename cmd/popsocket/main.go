package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/sonastea/popsocket/pkg/popsocket"
	"github.com/valkey-io/valkey-go"
)

// Run sets up and starts the PopSocket server.
func Run(ctx context.Context, valkey valkey.Client) error {
	mux := http.NewServeMux()

	ps, err := popsocket.New(
		valkey,
		popsocket.WithServeMux(mux),
		popsocket.WithAddress(os.Getenv("POPSOCKET_ADDR")),
	)
	if err != nil {
		return fmt.Errorf("Failed to create PopSocket: %w", err)
	}

	mux.HandleFunc("/", ps.ServeWsHandle)

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
