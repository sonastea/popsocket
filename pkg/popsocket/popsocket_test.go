package popsocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// TestLoadAllowedOrigins loads the env variable `ALLOWED_ORIGINS` and
// sanitizes/returns the set or default values.
func TestLoadAllowedOrigins(t *testing.T) {
	t.Parallel()

	originalAllowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	defer os.Setenv("ALLOWED_ORIGINS", originalAllowedOrigins) // Restore original value after test

	tests := []struct {
		name            string
		allowed         string
		expectedOrigins []string
	}{
		{
			name:            "No ALLOWED_allowed set",
			allowed:         "",
			expectedOrigins: []string{"localhost:3000"},
		},
		{
			name:            "Single origin set",
			allowed:         "example.com",
			expectedOrigins: []string{"example.com"},
		},
		{
			name:            "Multiple allowed set",
			allowed:         "example.com, kpoppop.com, localhost:8080",
			expectedOrigins: []string{"example.com", "kpoppop.com", "localhost:8080"},
		},
		{
			name:            "Origins with extra spaces",
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

// TestLoggingFunctions calls PopSocket's methods which are wrappers
// of slog's logging methods.
func TestLoggingFunctions(t *testing.T) {
	t.Parallel()

	ps, err := New()
	if err != nil {
		t.Fatalf("New PopSocket failed: %v", err)
	}

	tests := []struct {
		name     string
		log      func(msg string, args ...any)
		logLevel slog.Level
	}{
		{"LogDebug", ps.LogDebug, slog.LevelDebug},
		{"LogInfo", ps.LogInfo, slog.LevelInfo},
		{"LogError", ps.LogError, slog.LevelError},
		{"LogWarn", ps.LogWarn, slog.LevelWarn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				buf      strings.Builder
				logEntry map[string]interface{}
			)

			ps.logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			tt.log("test", "foo", "bar")

			err := json.Unmarshal([]byte(buf.String()), &logEntry)
			if err != nil {
				t.Fatalf("Failed to parse JSON log output: %v", err)
			}

			if logEntry["level"] != tt.logLevel.String() {
				t.Errorf("Expected log level %s, but got: %s", tt.logLevel, logEntry["level"])
			}
			if logEntry["msg"] != "test" {
				t.Errorf("Expected message 'Test message', but got: %s", logEntry["msg"])
			}
			if logEntry["foo"] != "bar" {
				t.Errorf("Expected 'key' field to be 'value', but got: %s", logEntry["key"])
			}
		})
	}
}

// TestNew ensures New() returns a valid PopSocket instance.
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

// TestNew_Options ensures New() can handle passed options
// and return a valid instance of PopSocket or error.
func TestNew_Options(t *testing.T) {
	t.Parallel()

	errMsg := fmt.Sprintf("mock option error")
	optionsWithError := func(ps *PopSocket) error {
		return errors.New(errMsg)
	}

	ps, err := New(optionsWithError)
	if err == nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err == nil {
		t.Fatalf("Expected error, but got none")
	}

	if err.Error() != errMsg {
		t.Fatalf("Expected error message '%s', but got '%s'", errMsg, err.Error())
	}

	if ps != nil {
		t.Fatalf("Expected PopSocket instance to be nil, but got %v", ps)
	}
}

// TestWithAddress ensures that the PopSocket instance
// sets and uses the specified address in httpServer.
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

// TestWithServeMux ensures that PopSocket's handler is set to use the provided ServeMux.
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

// TestServeWsHandle ensures that client connections are correctly managed
// and messages are processed from and to.
func TestServeWsHandle(t *testing.T) {
	t.Parallel()

	ps, err := New()
	if err != nil {
		t.Fatalf("New PopSocket failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(ps.ServeWsHandle))
	defer server.Close()

	t.Run("Websocket connection dialed", func(t *testing.T) {
		wsUrl := "ws" + strings.TrimPrefix(server.URL, "http")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, wsUrl, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket server: %v", err)
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		time.Sleep(100 * time.Millisecond)

		clients := ps.totalClients()

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

		clients = ps.totalClients()

		if clients != 0 {
			t.Errorf("Expected 0 clients after closing connection, got %d", clients)
		}
	})

	t.Run("Websocket accept errored", func(t *testing.T) {
		origin := "local.test"
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		req.Header.Set("Sec-WebSocket-Version", "13")
		req.Header.Set("Origin", origin)

		rec := httptest.NewRecorder()
		ps.ServeWsHandle(rec, req)

		if !bytes.Contains(rec.Body.Bytes(), []byte(origin)) {
			t.Errorf("Received body should have `local.test`, got %s", rec.Body.String())
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("Expected HTTP %d status code, got %d", http.StatusForbidden, rec.Code)
		}
	})
}

// TestStart runs Start() to make sure the server starts and handles shutdown properly.
func TestStart(t *testing.T) {
	t.Parallel()

	ps, err := New(WithAddress(":0"))
	if err != nil {
		t.Fatalf("New PopSocket failed: %v", err)
	}

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

// TestStartBroadcasting ensures connected clients are receiving messages from the PopSocket server.
func TestStartBroadcasting(t *testing.T) {
	t.Parallel()

	ps, err := New()
	if err != nil {
		t.Fatalf("New PopSocket failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(ps.ServeWsHandle))
	defer server.Close()

	wsUrl := "ws" + strings.TrimPrefix(server.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go ps.startBroadcasting(ctx)

	conn, _, err := websocket.Dial(ctx, wsUrl, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(100 * time.Millisecond)

	toClient := make(chan []byte)
	errChan := make(chan error)

	go func() {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			errChan <- err
		} else {
			toClient <- msg
		}
	}()

	select {
	case err := <-errChan:
		t.Fatalf("Failed to read message from server: %v", err)
	case msg := <-toClient:
		fmt.Printf("Received message: %s\n", string(msg))
	case <-ctx.Done():
		t.Fatalf("Test timed out waiting for a message")
	}
}

// TestStartWithFailure runs Start() with an error (occupied port) to determine if
// the server handles and returns an appropriate error.
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
	if err != nil {
		t.Fatalf("New PopSocket failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ps.Start(ctx)
	}()

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
