package popsocket

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sonastea/popsocket/pkg/db"
)

type SessionStore interface {
	Find(ctx context.Context, sid string) (Session, error)
	HasExpired(ctx context.Context, sid string) (bool, error)
	UserFromDiscordID(ctx context.Context, discordID string) (int32, error)
}

type sessionStore struct {
	db db.DB
}

type User struct {
	ID        int32   `json:"id,omitempty"`
	DiscordID *string `json:"discordId,omitempty"`
}

type Passport struct {
	User User `json:"user"`
}

type SessionData struct {
	Passport Passport `json:"passport"`
}

type Session struct {
	SID       string      `json:"sid"`
	ExpiresAt time.Time   `json:"expiresAt"`
	Data      SessionData `json:"data"`
}

const (
	SESSION_EXPIRED        = "Session has expired. Please log in again."
	SESSION_ERROR          = "Unexpected error with your session. Please log in again."
	SESSION_UNAUTHORIZED   = "Unauthorized: Missing or invalid session."
	SESSION_MISSING_COOKIE = "Missing session cookie in request headers."
)

// NewSessionStore creates a new instance of sessionStore.
func NewSessionStore(db db.DB) *sessionStore {
	return &sessionStore{db: db}
}

// Find queries the database for a session matching the sid and populates
// the context with the client's userIDKey and discordIDKey.
func (ss *sessionStore) Find(ctx context.Context, sid string) (Session, error) {
	var session Session
	var jsonData json.RawMessage

	query := `SELECT s.sid, s.data, s."expiresAt" FROM "Session" s WHERE s.sid = $1`
	err := ss.db.QueryRow(ctx, query, sid).Scan(&session.SID, &jsonData, &session.ExpiresAt)
	if err != nil {
		Logger().Error(fmt.Sprintf("%+v", err))
		return Session{}, err
	}

	if time.Now().After(session.ExpiresAt) {
		return Session{}, fmt.Errorf(SESSION_EXPIRED)
	}

	err = json.Unmarshal(jsonData, &session.Data)
	if err != nil {
		Logger().Error(fmt.Sprintf("SessionStore.Find Unmarshal Error: %s", err.Error()))
		return Session{}, fmt.Errorf(SESSION_ERROR)
	}

	return session, nil
}

// HasExpired queries the database for a session matching the sid
// and returns a bool and an error indicating the session's expiration status.
func (ss *sessionStore) HasExpired(ctx context.Context, sid string) (bool, error) {
	var expiresAt time.Time

	query := `SELECT s."expiresAt" FROM "Session" s WHERE s.sid = $1`
	err := ss.db.QueryRow(ctx, query, sid).Scan(&expiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil // Consider missing session as expired.
		}
		Logger().Debug(fmt.Sprintf("%+v", err))
		return false, err
	}

	return time.Now().After(expiresAt), nil
}

// UserFromDiscordID finds the client's user id to fulfill the client's id field.
// Every client has a local account with kpopppop regardless of the user's login method.
// Therefore, we use client.ID() because their userID should always be present.
func (ss *sessionStore) UserFromDiscordID(ctx context.Context, discordID string) (int32, error) {
	var userID int32
	query := `SELECT id FROM "DiscordUser" WHERE "discordId" = $1`

	err := ss.db.QueryRow(ctx, query, discordID).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf(SESSION_UNAUTHORIZED)
	}

	return userID, nil
}
