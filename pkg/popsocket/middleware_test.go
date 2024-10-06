package popsocket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCheckSession_WithCookie calls `checkSession` middleware with a connect.sid cookie to
// ensure the next HandlerFunc is called afterward.
func TestCheckSession_WithCookie(t *testing.T) {
	expectedBody := "next"
	handlerToTest := checkSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, expectedBody)
	}))

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "connect.sid", Value: "valid-session-id"})

	rr := httptest.NewRecorder()
	handlerToTest.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, status)
	}

	if rr.Body.String() != expectedBody {
		t.Errorf("Expected response body '%s', got '%s'", expectedBody, rr.Body.String())
	}
}

// TestCheckSession_WithCookie calls `checkSession` middleware without a connect.sid cookie to
// ensure the http request is closed with the StatusUnauthorized code.
func TestCheckSession_WithoutCookie(t *testing.T) {
	expectedBody := "Unauthorized: Missing or invalid session\n"
	handlerToTest := checkSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	rr := httptest.NewRecorder()
	handlerToTest.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, status)
	}

	if rr.Body.String() != expectedBody {
		t.Errorf("Expected response body '%s', got '%s'", expectedBody, rr.Body.String())
	}
}
