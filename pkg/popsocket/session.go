package popsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sonastea/popsocket/pkg/db"
)

type SessionStore interface {
	Find(ctx context.Context, sid string) (context.Context, error)
	UserFromDiscordID(ctx context.Context, data SessionData) (context.Context, error)
}

type sessionStore struct {
	db db.DB
}

type User struct {
	ID        *string `json:"id,omitempty"`
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
	SESSION_EXPIRED = "Session has expired. Please log in again."
	SESSION_ERROR   = "Unexpected error with your session. Please log in again."
)

// NewSessionStore creates a new instance of sessionStore.
func NewSessionStore(db db.DB) *sessionStore {
	return &sessionStore{db: db}
}

// Find queries the database for a session matching the sid and populates
// the context with the client's userIDKey and discordIDKey.
func (ss *sessionStore) Find(ctx context.Context, sid string) (context.Context, error) {
	var session Session
	var jsonData json.RawMessage

	query := `SELECT s.sid, s.data, s."expiresAt" FROM "Session" s WHERE s.sid = $1`
	err := ss.db.QueryRow(ctx, query, sid).Scan(&session.SID, &jsonData, &session.ExpiresAt)
	if err != nil {
		Logger().Error(fmt.Sprintf("%+v", err))
		return nil, err
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf(SESSION_EXPIRED)
	}

	err = json.Unmarshal(jsonData, &session.Data)
	if err != nil {
		Logger().Error(fmt.Sprintf("SessionStore.Find Unmarshal Error: %s", err.Error()))
		return nil, fmt.Errorf(SESSION_ERROR)
	}

	// UserID present, return with context here
	if session.Data.Passport.User.ID != nil {
		ctx = context.WithValue(ctx, USER_ID_KEY, &session.Data.Passport.User.ID)
		return ctx, nil
	}

	// DiscordID, get client's UserID
	return ss.UserFromDiscordID(ctx, session.Data)
}

// userFromDiscordID finds the client's user id to fulfill the client's id field.
// Every client has a local account with kpopppop regardless of the user's login method.
// Therefore, we use client.ID() because their userID should always be present.
func (ss *sessionStore) UserFromDiscordID(ctx context.Context, data SessionData) (context.Context, error) {
	query := `SELECT id FROM "DiscordUser" WHERE "discordId" = $1`

	discordID := *data.Passport.User.DiscordID
	var userID int
	err := ss.db.QueryRow(ctx, query, discordID).Scan(&userID)
	if err != nil {
		Logger().Warn(fmt.Sprintf("%+v", err))
		return nil, fmt.Errorf("Failed to find user with discord id %s", discordID)
	}

	ctx = context.WithValue(ctx, USER_ID_KEY, userID)
	ctx = context.WithValue(ctx, DISCORD_ID_KEY, discordID)

	return ctx, nil
}
