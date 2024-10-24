package popsocket

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sonastea/popsocket/pkg/testutil"
)

func TestNewSessionMiddleware(t *testing.T) {
	sm := NewSessionMiddleware(&MockSessionStore{})

	if _, ok := interface{}(sm).(SessionMiddleware); !ok {
		t.Fatalf("Expected variable to be of type SessionMiddleware")
	}
}

func TestValidateCookie(t *testing.T) {
	t.Parallel()

	secretKey, err := testutil.SetRandomTestSecretKey()
	if err != nil {
		t.Fatalf("Unable to set random test secret key: %s", err)
	}

	expectedSID := "s9foo"
	expectedDiscordID := "9"
	expectedUserID := int32(9)
	cookie, err := testutil.CreateSignedCookie(expectedSID, secretKey)
	if err != nil {
		t.Fatalf("Unable to create signed cookie: %s", err)
	}

	t.Run("Valid Cookie and Session With UserID", func(t *testing.T) {
		mockStore := &MockSessionStore{
			FindFunc: func(ctx context.Context, sid string) (Session, error) {
				return Session{
					SID: expectedSID,
					Data: SessionData{
						Passport: Passport{
							User: User{
								ID:        expectedUserID,
								DiscordID: &expectedDiscordID,
							},
						},
					},
				}, nil
			},
		}
		sm := NewSessionMiddleware(mockStore)

		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "connect.sid", Value: cookie})
		rr := httptest.NewRecorder()

		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value(USER_ID_KEY)
			if userID != expectedUserID {
				t.Fatalf("Expected UserID %v, got %v", expectedUserID, userID)
			}
		})

		handler := sm.ValidateCookie(nextHandler)
		handler.ServeHTTP(rr, req)
	})

	t.Run("Valid Cookie and Session With DiscordID", func(t *testing.T) {
		mockStore := &MockSessionStore{
			FindFunc: func(ctx context.Context, sid string) (Session, error) {
				return Session{
					SID: expectedSID,
					Data: SessionData{
						Passport: Passport{
							User: User{
								ID:        0,
								DiscordID: &expectedDiscordID,
							},
						},
					},
				}, nil
			},
			UserFromDiscordIDFunc: func(ctx context.Context, discordID string) (int32, error) {
				if discordID == expectedDiscordID {
					return expectedUserID, nil
				}
				return 0, fmt.Errorf(SESSION_UNAUTHORIZED)
			},
		}
		sm := NewSessionMiddleware(mockStore)

		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "connect.sid", Value: cookie})
		rr := httptest.NewRecorder()

		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value(USER_ID_KEY)
			if userID != expectedUserID {
				t.Fatalf("Expected UserID %v, got %v", expectedUserID, userID)
			}
		})

		handler := sm.ValidateCookie(nextHandler)
		handler.ServeHTTP(rr, req)
	})
	t.Run("Unable to extract session id", func(t *testing.T) {
		mockStore := &MockSessionStore{
			FindFunc: func(ctx context.Context, sid string) (Session, error) {
				return Session{
					SID: "s9-fail",
				}, nil
			},
		}
		sm := NewSessionMiddleware(mockStore)

		req, err := http.NewRequest("GET", "/test", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: "connect.sid", Value: "s:BadCookie"})

		rr := httptest.NewRecorder()
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Next handler should not have been called")
		})

		handler := sm.ValidateCookie(nextHandler)
		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
		}

		if strings.TrimSpace(rr.Body.String()) != SESSION_UNAUTHORIZED {
			t.Errorf("Expected error message '%s', got '%s'.", SESSION_UNAUTHORIZED, rr.Body.String())
		}
	})

	t.Run("Invalid Session ID", func(t *testing.T) {
		mockStore := &MockSessionStore{
			FindFunc: func(ctx context.Context, sid string) (Session, error) {
				return Session{SID: "invalid-sid"}, fmt.Errorf(SESSION_ERROR)
			},
		}
		sm := NewSessionMiddleware(mockStore)

		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "connect.sid", Value: expectedDiscordID})

		rr := httptest.NewRecorder()
		handler := sm.ValidateCookie(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		}))
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %v", rr.Code)
		}
	})
}
