package popsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sonastea/popsocket/pkg/db"
)

type sessionStore struct {
	db db.Database
}

type User struct {
	ID        string `json:"id,omitempty"`
	DiscordID string `json:"discordId,omitempty"`
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

// NewSessionStore creates a new instance of sessionStore.
func NewSessionStore(db db.Database) *sessionStore {
	return &sessionStore{db: db}
}

// Find queries the database for a session matching the sid.
func (ss *sessionStore) Find(ctx context.Context, sid string) (*Session, error) {
	var session Session
	query := `
      SELECT s.sid, s.data, s."expiresAt"
      FROM "Session" s
      WHERE s.sid = $1;
      `
	var jsonData json.RawMessage
	err := ss.db.Pool().QueryRow(ctx, query, sid).Scan(&session.SID, &jsonData, &session.ExpiresAt)
	if err != nil {
		return nil, err
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("Session has expired. Please log in again.")
	}

	err = json.Unmarshal(jsonData, &session.Data)
	if err != nil {
		return nil, fmt.Errorf("Unexpected error with your session. Please log in again.")
	}

	return &session, nil
}
