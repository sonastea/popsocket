package popsocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	"github.com/sonastea/popsocket/pkg/testutil"
	"github.com/valkey-io/valkey-go"
)

// TestLoadAllowedOrigins loads the env variable `ALLOWED_ORIGINS` and
// sanitizes/returns the set or default values.
func TestLoadAllowedOrigins(t *testing.T) {
	originalAllowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	defer t.Setenv("ALLOWED_ORIGINS", originalAllowedOrigins) // Restore original value after test

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
			t.Setenv("ALLOWED_ORIGINS", tt.allowed)

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
	s := miniredis.RunT(t)
	defer s.Close()

	t.Setenv("REDIS_URL", s.Addr())

	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	ps, err := New(valkey)
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
	s := miniredis.RunT(t)
	defer s.Close()

	t.Setenv("REDIS_URL", s.Addr())
	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	ps, err := New(valkey)
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
	s := miniredis.RunT(t)
	t.Setenv("REDIS_URL", s.Addr())

	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	errMsg := fmt.Sprintf("mock option error")
	optionsWithError := func(ps *PopSocket) error {
		return errors.New(errMsg)
	}

	ps, err := New(valkey, optionsWithError)
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
	s := miniredis.RunT(t)
	defer s.Close()

	t.Setenv("REDIS_URL", s.Addr())

	addr := ":8080"
	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	ps, err := New(valkey, WithAddress(addr))
	if err != nil {
		t.Fatalf("New() with WithAddress failed: %v", err)
	}

	if ps.httpServer.Addr != addr {
		t.Errorf("Expected address %s, got %s", addr, ps.httpServer.Addr)
	}
}

// TestWithServeMux ensures that PopSocket's handler is set to use the provided ServeMux.
func TestWithServeMux(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	t.Setenv("REDIS_URL", s.Addr())

	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	customMux := http.NewServeMux()
	ps, err := New(valkey, WithServeMux(customMux))
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
	s := miniredis.RunT(t)
	defer s.Close()

	os.Setenv("REDIS_URL", s.Addr())
	secretKey, err := testutil.SetRandomTestSecretKey()
	if err != nil {
		t.Fatalf("Unable to set random test secret key: %s", err)
	}

	expectedSID := "foo"
	cookie, err := testutil.CreateSignedCookie(expectedSID, secretKey)
	if err != nil {
		t.Fatalf("Unable to create signed cookie: %s", err)
	}

	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	ps, err := New(valkey)
	if err != nil {
		t.Fatalf("New PopSocket failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(ps.ServeWsHandle))
	defer server.Close()

	t.Run("Websocket connection dialed", func(t *testing.T) {
		wsUrl := "ws" + strings.TrimPrefix(server.URL, "http")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ctx = context.WithValue(ctx, userIDKey, "9")
		defer cancel()

		header := http.Header{}
		header.Add("Cookie", fmt.Sprintf("connect.sid=%s", cookie))

		conn, _, err := websocket.Dial(ctx, wsUrl, &websocket.DialOptions{
			HTTPHeader: header,
		})
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket server: %v", err)
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		time.Sleep(100 * time.Millisecond)

		clients := ps.totalClients()

		if clients != 1 {
			t.Errorf("Expected 1 client, got %d", clients)
		}

		fmt.Printf("client: %+v \n", ctx.Value(userIDKey).(string))

		psMessage := EventMessage{Event: EventMessageType.Connect, Content: "ping!"}
		m, err := json.Marshal(psMessage)
		err = conn.Write(ctx, websocket.MessageText, m)
		if err != nil {
			t.Fatalf("Failed to send message to server: %v", err)
		}

		_, msg, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("Failed to read message from server: %v", err)
		}

		var message EventMessage
		err = json.Unmarshal(msg, &message)
		if err != nil {
			t.Fatalf("Failed to unmarshal message: %v, got %s", err, string(msg))
		}

		if message.Event != EventMessageType.Connect {
			t.Errorf("Expected event '%s', got '%s'", EventMessageType.Connect, message.Event)
		}

		cancel()
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
	s := miniredis.RunT(t)
	defer s.Close()

	t.Setenv("REDIS_URL", s.Addr())

	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	ps, err := New(valkey, WithAddress(":0"))
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

// TestStartWithFailure runs Start() with an error (occupied port) to determine if
// the server handles and returns an appropriate error.
func TestStartWithFailure(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	defer listener.Close()

	s := miniredis.RunT(t)
	defer s.Close()

	t.Setenv("REDIS_URL", s.Addr())

	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	// Now create a PopSocket instance trying to use the occupied port
	ps, _ := New(valkey, WithAddress(fmt.Sprintf(":%d", port)))
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

// TestNewWithCustomTimeout runs Start() to make sure the server starts and handles shutdown properly.
func TestNewWithCustomTimeout(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	t.Setenv("REDIS_URL", s.Addr())

	valkey, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	expectedTimeout := 9 * time.Second
	ps, err := New(valkey, WithAddress(":0"), WithWriteTimeout(expectedTimeout), WithReadTimeout(expectedTimeout), WithIdleTimeout(expectedTimeout))
	if err != nil {
		t.Fatalf("New PopSocket failed: %v", err)
	}

	if ps.httpServer.WriteTimeout != expectedTimeout {
		t.Fatalf("Expected write timeout of %v, got %s", expectedTimeout, ps.httpServer.WriteTimeout)
	}

	if ps.httpServer.ReadTimeout != expectedTimeout {
		t.Fatalf("Expected read timeout of %v, got %s", expectedTimeout, ps.httpServer.ReadTimeout)
	}

	if ps.httpServer.IdleTimeout != expectedTimeout {
		t.Fatalf("Expected idle timeout of %v, got %s", expectedTimeout, ps.httpServer.IdleTimeout)
	}
}

// TestSetupRoutes tests that the SetupRoutes function correctly
// registers the checkSession middleware for the ServeWsHandle.
func TestSetupRoutes(t *testing.T) {
	ps := &PopSocket{}

	mux := http.NewServeMux()
	err := ps.SetupRoutes(mux)
	if err != nil {
		t.Fatalf("Expected no error, but got %v", err)
	}

	ts := httptest.NewServer(mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Expected no error when making GET request, but got %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("Expected no error reading body, got %v", err)
	}

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status code 401, but got %d", res.StatusCode)
	}

	expectedBody := "Unauthorized: Missing or invalid session"
	if string(body) == expectedBody {
		t.Errorf("Expected response body '%s', got '%s'", expectedBody, string(body))
	}
}

// TestSetupRoutes_WithErr tests that the SetupRoutes function correctly
// registers the checkSession middleware for the ServeWsHandle.
func TestSetupRoutes_WithErr(t *testing.T) {
	ps := &PopSocket{}

	mux := http.NewServeMux()
	err := ps.SetupRoutes(mux)
	if err != nil {
		t.Fatalf("Expected no error, but got %v", err)
	}

	ts := httptest.NewServer(mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Expected no error when making GET request, but got %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("Expected no error reading body, got %v", err)
	}

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status code 401, but got %d", res.StatusCode)
	}

	expectedBody := "Unauthorized: Missing or invalid session"
	if string(body) == expectedBody {
		t.Errorf("Expected response body '%s', got '%s'", expectedBody, string(body))
	}
}
