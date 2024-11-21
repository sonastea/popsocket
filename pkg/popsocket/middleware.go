package popsocket

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sonastea/popsocket/pkg/config"
	"github.com/sonastea/popsocket/pkg/util"
)

type contextKey string

const (
	DISCORD_ID_KEY contextKey = "discordID"
	USER_ID_KEY    contextKey = "userID"
	SID_KEY        contextKey = "SID"
)

type SessionMiddleware struct {
	sessionStore SessionStore
}

// NewSessionMiddleware returns an instance of SessionMiddleware.
func NewSessionMiddleware(store SessionStore) SessionMiddleware {
	return SessionMiddleware{
		store,
	}
}

// ValidateCookie checks and verifies the session id cookie is present
// and legitimate before handing over to the next chain of HandlerFunc.
func (sm *SessionMiddleware) ValidateCookie(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := sm.checkCookiePresence(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		sid, err := sm.extractSessionID(cookie)
		if err != nil {
			http.Error(w, SESSION_UNAUTHORIZED, http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		session, err := sm.sessionStore.Find(ctx, sid)
		if err != nil {
			http.Error(w, SESSION_UNAUTHORIZED, http.StatusUnauthorized)
			return
		}

		ctx = sm.bindToContext(ctx, session, sid)
		next(w, r.WithContext(ctx))
	}
}

// bindToContext adds the client's session data and sid
// (session id in database) to the request context.
func (sm *SessionMiddleware) bindToContext(ctx context.Context, session Session, sid string) context.Context {
	if session.Data.Passport.User.ID != 0 {
		ctx = context.WithValue(ctx, USER_ID_KEY, session.Data.Passport.User.ID)
	} else if session.Data.Passport.User.DiscordID != nil {
		discordID := *session.Data.Passport.User.DiscordID
		userID, err := sm.sessionStore.UserFromDiscordID(ctx, discordID)
		if err == nil {
			ctx = context.WithValue(ctx, USER_ID_KEY, userID)
			ctx = context.WithValue(ctx, DISCORD_ID_KEY, discordID)
		}
	}

	ctx = context.WithValue(ctx, SID_KEY, sid)

	return ctx
}

// checkCookiePresence only checks that the http.Request has the proper session cookie.
func (sm *SessionMiddleware) checkCookiePresence(r *http.Request) (string, error) {
	cookie, err := r.Cookie("connect.sid")
	if err != nil || cookie.Value == "" {
		return "", fmt.Errorf(SESSION_MISSING_COOKIE)
	}

	return cookie.Value, nil
}

// extractSessionID accepts the cookie value to decode it to return the client's sid or errors.
func (sm *SessionMiddleware) extractSessionID(cookie string) (string, error) {
	sid, err := util.DecodeCookie(cookie, config.ENV.SESSION_SECRET_KEY.Value)
	if err != nil {
		return "", fmt.Errorf(SESSION_UNAUTHORIZED)
	}

	return sid, nil
}
