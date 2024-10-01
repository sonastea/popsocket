package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/sonastea/popsocket/pkg/popsocket"
	"github.com/valkey-io/valkey-go"
)

// TestRun ensures popsocket's cmd entry point properly runs
func TestRun(t *testing.T) {
	t.Parallel()

	os.Setenv("POPSOCKET_ADDR", ":8989")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	valkey, err := popsocket.NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	go func() {
		errCh <- Run(ctx, valkey)
	}()

	time.Sleep(500 * time.Millisecond)

	wsURL := "ws://127.0.0.1:8989/"

	wsConn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{})
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer wsConn.CloseNow()

	if wsConn == nil {
		t.Fatal("Expected a valid WebSocket connection, got nil")
	}

	psMessage := popsocket.Message{Event: popsocket.MessageType.Connect, Content: "ping!"}
	m, err := json.Marshal(psMessage)
	err = wsConn.Write(ctx, websocket.MessageText, m)
	if err != nil {
		t.Fatalf("Failed to send message to WebSocket server: %v", err)
	}

	_, msg, err := wsConn.Read(ctx)
	if err != nil {
		t.Fatalf("Failed to read message from WebSocket server: %v", err)
	}

	expectedMessage := `{"event":"CONNECT","content":"pong!"}`
	if string(msg) != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, msg)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Server encountered an error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for server shutdown")
	}
}
