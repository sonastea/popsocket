package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sonastea/popsocket/pkg/popsocket"
)

func main() {
	mux := http.NewServeMux()

	ps, err := popsocket.New(
		popsocket.WithServeMux(mux),
	)
	if err != nil {
		log.Fatalln(err)
	}

	mux.HandleFunc("/", ps.ServeWsHandle)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stopSig := make(chan os.Signal, 1)
	signal.Notify(stopSig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	stop := make(chan os.Signal, 1)

	go func() {
		if err := ps.Start(ctx); err != nil {
			log.Printf("Server error: %v", err)
		}
		close(stop)
	}()

	select {
	case <-stopSig:
		log.Println("Received interrupt signal. Shutting down...")
		cancel()
	case <-stop:
		log.Println("PopSocket server stopped unexpectedly")
	}

	<-stop
	log.Println("PopSocket server shutdown complete")
}
