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
	userIDKey    contextKey = "userID"
	discordIDKey contextKey = "discordID"
)

// checkCookie looks at the request headers to verify a connect.sid cookie is present
// before handing over to the next chain of HandlerFunc(s)
func (p *PopSocket) checkCookie(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("connect.sid")
		if err != nil || cookie.Value == "" {
			http.Error(w, "Unauthorized: Missing or invalid session", http.StatusUnauthorized)
			Logger().Error(fmt.Sprintf("[CheckSession]: %v", err))
			return
		}

		sid, err := util.DecodeCookie(cookie.Value, config.ENV.SESSION_SECRET_KEY.Value)
		if err != nil {
			http.Error(w, "Unauthorized: Missing or invalid session", http.StatusUnauthorized)
			Logger().Error(fmt.Sprintf("[DecodeCookie] error: %+v", err))
			return
		}

		ctx := r.Context()
		session, err := p.SessionStore.Find(ctx, sid)
		if err != nil {
			http.Error(w, "Unauthorized: Missing or invalid session", http.StatusUnauthorized)
			Logger().Error(fmt.Sprintf("[SessionStore.Find] error: %+v", err))
			return
		}

		ctx = context.WithValue(ctx, "sid", session)

		if session.Data.Passport.User.ID != "" {
			ctx = context.WithValue(ctx, userIDKey, session.Data.Passport.User.ID)
		} else if session.Data.Passport.User.DiscordID != "" {
			ctx = context.WithValue(ctx, discordIDKey, session.Data.Passport.User.DiscordID)
		}

		next(w, r.WithContext(ctx))
	}
}
