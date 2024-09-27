package popsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestLoadAllowedOrigins(t *testing.T) {
	t.Parallel()

	originalAllowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	defer os.Setenv("ALLOWED_ORIGINS", originalAllowedOrigins) // Restore original value after test

	tests := []struct {
		name           string
		allowed         string
		expectedOrigins []string
	}{
		{
			name:           "No ALLOWED_allowed set",
			allowed:         "",
			expectedOrigins: []string{"localhost:3000"},
		},
		{
			name:           "Single origin set",
			allowed:         "example.com",
			expectedOrigins: []string{"example.com"},
		},
		{
			name:           "Multiple allowed set",
			allowed:         "example.com, kpoppop.com, localhost:8080",
			expectedOrigins: []string{"example.com", "kpoppop.com", "localhost:8080"},
		},
		{
			name:           "Origins with extra spaces",
			allowed:         " example.com , kpoppop.com , localhost:8080 ",
			expectedOrigins: []string{"example.com", "kpoppop.com", "localhost:8080"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("ALLOWED_ORIGINS", tt.allowed)

			loadAllowedOrigins()

			if !reflect.DeepEqual(AllowedOrigins, tt.expectedOrigins) {
				t.Errorf("Expected AllowedOrigins to be %v, got %v", tt.expectedOrigins, AllowedOrigins)
			}
		})
	}

}

func TestNew(t *testing.T) {
	t.Parallel()

	ps, err := New()

	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if ps == nil {
		t.Fatal("New() returned nil PopSocket")
	}

	if ps.clients == nil {
		t.Error("clients map not initialized")
	}

	if ps.httpServer == nil {
		t.Error("httpServer not initialized")
	}

	if ps.logger == nil {
		t.Error("logger not initialized")
	}
}

func TestWithAddress(t *testing.T) {
	t.Parallel()

	addr := ":8080"
	ps, err := New(WithAddress(addr))

	if err != nil {
		t.Fatalf("New() with WithAddress failed: %v", err)
	}

	if ps.httpServer.Addr != addr {
		t.Errorf("Expected address %s, got %s", addr, ps.httpServer.Addr)
	}
}

func TestWithServeMux(t *testing.T) {
	t.Parallel()

	customMux := http.NewServeMux()
	ps, err := New(WithServeMux(customMux))
	if err != nil {
		t.Fatalf("New() with WithServeMux failed: %v", err)
	}
	if ps.httpServer.Handler != customMux {
		t.Error("Custom ServeMux not set correctly")
	}
}

func TestServeWsHandle(t *testing.T) {
	t.Parallel()

	ps, _ := New()
	server := httptest.NewServer(http.HandlerFunc(ps.ServeWsHandle))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(100 * time.Millisecond)

	ps.mu.Lock()
	clients := len(ps.clients)
	ps.mu.Unlock()

	if clients != 1 {
		t.Errorf("Expected 1 client, got %d", clients)
	}

	err = conn.Write(ctx, websocket.MessageText, []byte("Hello, server!"))
	if err != nil {
		t.Fatalf("Failed to send message to server: %v", err)
	}

	_, message, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Failed to read message from server: %v", err)
	}

	var msg Message
	err = json.Unmarshal(message, &msg)
	if err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if msg.Event != "conversations" {
		t.Errorf("Expected event 'conversations', got '%s'", msg.Event)
	}

	conn.Close(websocket.StatusNormalClosure, "")
	time.Sleep(100 * time.Millisecond)

	ps.mu.Lock()
	clients = len(ps.clients)
	ps.mu.Unlock()

	if clients != 0 {
		t.Errorf("Expected 0 clients after closing connection, got %d", clients)
	}
}

func TestStart(t *testing.T) {
	t.Parallel()

	ps, _ := New(WithAddress(":0"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ps.Start(ctx)
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Start() failed: %v", err)
		}
	case <-time.After(1 * time.Second):
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Unexpected error during shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server didn't shut down within the expected time")
	}
}

func TestStartWithFailure(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	defer listener.Close()

	// Now create a PopSocket instance trying to use the occupied port
	ps, _ := New(WithAddress(fmt.Sprintf(":%d", port)))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ps.Start(ctx)
	}()

	// Wait for the server to start or timeout
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Expected an error due to occupied port, but got nil")
		}
		// If we reach here, the test passes because we got an error as expected
		t.Logf("Received expected error: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for server to return an error")
	}
}
