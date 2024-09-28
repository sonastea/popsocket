package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/sonastea/popsocket/pkg/popsocket"
)

// Run sets up and starts the PopSocket server.
func Run(ctx context.Context) error {
	mux := http.NewServeMux()

	ps, err := popsocket.New(
		popsocket.WithServeMux(mux),
		popsocket.WithAddress(os.Getenv("POPSOCKET_ADDR")),
	)
	if err != nil {
		return fmt.Errorf("failed to create PopSocket: %w", err)
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
	if err := Run(ctx); err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}
}
