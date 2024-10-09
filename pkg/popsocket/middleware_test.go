package popsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sonastea/popsocket/pkg/testutil"
)

type FindFuncType func(ctx context.Context, sid string) (*Session, error)

type MockSessionStore struct {
	FindFunc FindFuncType
}

func (m *MockSessionStore) Find(ctx context.Context, sid string) (*Session, error) {
	if m.FindFunc != nil {
		return m.FindFunc(ctx, sid)
	}
	return nil, fmt.Errorf("Session has expired. Please log in again.")
}

// TestCheckSession_WithCookie calls `checkCookie` middleware with a connect.sid cookie to
// ensure the next HandlerFunc is called afterward.
func TestCheckCookie_WithCookie(t *testing.T) {
	secretKey, err := testutil.SetRandomTestSecretKey()
	if err != nil {
		t.Fatalf("Unable to set random test secret key: %s", err)
	}

	expectedUserID := "9"
	expectedSID := "foo"
	cookie, err := testutil.CreateSignedCookie(expectedSID, secretKey)
	if err != nil {
		t.Fatalf("Unable to create signed cookie: %s", err)
	}

	mockStore := &MockSessionStore{
		FindFunc: func(ctx context.Context, sid string) (*Session, error) {
			return &Session{
				SID: expectedSID,
				Data: SessionData{
					Passport: Passport{
						User: User{
							ID: expectedUserID,
						},
					},
				},
				ExpiresAt: time.Now().Add(5 * time.Minute),
			}, nil
		},
	}
	p := &PopSocket{
		SessionStore: mockStore,
	}

	handlerToTest := p.checkCookie(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := r.Context().Value("sid").(*Session)
		if !ok {
			http.Error(w, "Unable to retrieve session from context", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(session)
	}))

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "connect.sid", Value: cookie})

	rr := httptest.NewRecorder()
	handlerToTest.ServeHTTP(rr, req)

	var session Session
	json.Unmarshal(rr.Body.Bytes(), &session)

	if session.SID != expectedSID {
		t.Errorf("Expected sid '%s', got '%s'", expectedSID, session.SID)
	}

	if session.Data.Passport.User.ID != expectedUserID {
		t.Errorf("Expected userID '%s', got '%s'", expectedUserID, session.Data.Passport.User.ID)
	}
}

// TestCheckCookie_WithoutCookie calls `checkCookie` middleware without a connect.sid cookie to
// ensure the http request is closed with the StatusUnauthorized code.
func TestCheckCookie_WithoutCookie(t *testing.T) {
	p := &PopSocket{}
	expectedBody := "Unauthorized: Missing or invalid session\n"

	handlerToTest := p.checkCookie(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
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
