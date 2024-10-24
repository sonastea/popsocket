package popsocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	mock_db "github.com/sonastea/popsocket/internal/mock"
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

// TestServeWs ensures that ServeWs is upgrading a client's http connect to a websocket.
func TestServeWs(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	t.Setenv("REDIS_URL", s.Addr())
	secretKey, err := testutil.SetRandomTestSecretKey()
	if err != nil {
		t.Fatalf("Unable to set random test secret key: %s", err)
	}

	expectedSID := "foo"
	cookie, err := testutil.CreateSignedCookie(expectedSID, secretKey)
	if err != nil {
		t.Fatalf("Unable to create signed cookie: %s", err)
	}

	vk, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
	if err != nil {
		t.Fatalf("Expected new valkey client, got %s", err)
	}

	customMux := http.NewServeMux()
	ps, err := New(vk, WithAddress(":8080"), WithServeMux(customMux))
	if err != nil {
		t.Fatalf("New PopSocket failed: %v", err)
	}

	wsUrl := "ws://localhost:8080"
	customMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveWs(ps, w, r)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	ctx = context.WithValue(ctx, USER_ID_KEY, "9")
	defer cancel()

	go func() {
		if err := ps.Start(ctx); err != nil {
			t.Error("Unable to start PopSocket server.")
		}
	}()

	header := http.Header{}
	header.Add("Cookie", fmt.Sprintf("connect.sid=%s", cookie))

	conn, _, err := websocket.Dial(ctx, wsUrl, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	clients := ps.totalClients()
	if clients != 1 {
		t.Errorf("Expected 1 client, got %d", clients)
	}

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

	conn.Close(websocket.StatusNormalClosure, "Disconnecting from websocket, should be 0 clients connected.")
	time.Sleep(100 * time.Millisecond)

	clients = ps.totalClients()
	if clients != 0 {
		t.Errorf("Expected 0 clients after closing connection, got %d", clients)
	}
}

// TestWithOpts ensures that the PopSocket instance sets and uses
// the passed WithXXX values.
func TestWithOpts(t *testing.T) {
	t.Run("With Address", func(t *testing.T) {
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
	})

	t.Run("With ServeMux", func(t *testing.T) {
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
	})

	t.Run("With MessageService", func(t *testing.T) {
		s := miniredis.RunT(t)
		defer s.Close()

		t.Setenv("REDIS_URL", s.Addr())

		vk, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
		if err != nil {
			t.Fatalf("Expected new valkey client, got %s", err)
		}
		messageStore := NewMessageStore(mock_db.New())
		messageService := NewMessageService(messageStore)

		// TODO: CONVERTS MessageStore to MessageService like sessionmiddleware as business layer
		// while MessageStore handles database layer.
		ps, err := New(vk, WithMessageService(messageService))
		if err != nil {
			t.Fatalf("New() with WithMessageService failed: %v", err)
		}

		if ps.MessageService != messageService {
			t.Errorf("Expected message service %v, got %v", messageService, ps.MessageService)
		}
	})

	t.Run("With SessionMiddleware", func(t *testing.T) {
		s := miniredis.RunT(t)
		defer s.Close()

		t.Setenv("REDIS_URL", s.Addr())

		vk, err := NewValkeyClient(valkey.ClientOption{DisableCache: true})
		if err != nil {
			t.Fatalf("Expected new valkey client, got %s", err)
		}
		sessionStore := NewSessionStore(mock_db.New())
		sessionMiddleware := NewSessionMiddleware(sessionStore)

		ps, err := New(vk, WithSessionMiddleware(sessionMiddleware))
		if err != nil {
			t.Fatalf("New() with SessionMiddleware failed: %v", err)
		}

		if ps.SessionMiddleware != sessionMiddleware {
			t.Errorf("Expected session middleware %v, got %v", sessionMiddleware, ps.SessionMiddleware)
		}
	})
}
