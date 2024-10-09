package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	"github.com/sonastea/popsocket/pkg/popsocket"
	"github.com/sonastea/popsocket/pkg/testutil"
	"github.com/valkey-io/valkey-go"
)

// TestRun ensures popsocket's cmd entry point properly runs
func TestRun(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	t.Setenv("POPSOCKET_ADDR", ":8989")
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
	header := http.Header{}
	header.Add("Cookie", fmt.Sprintf("connect.sid=%s", cookie))

	_, r, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		defer r.Body.Close()
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Fatalf("Failed to read response body: %v", readErr)
		}

		expectedBody := "Unauthorized: Missing or invalid session\n"
		if string(body) != expectedBody {
			t.Fatalf("Expected body '%v', got '%v'", expectedBody, string(body))
		}
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
