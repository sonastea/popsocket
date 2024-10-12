package popsocket

import (
	"fmt"
	"net/http"

	"github.com/sonastea/popsocket/pkg/config"
	"github.com/sonastea/popsocket/pkg/util"
)

type contextKey string

const (
	USER_ID_KEY    contextKey = "userID"
	DISCORD_ID_KEY contextKey = "discordID"

	unauthorized_session_err = "Unauthorized: Missing or invalid session"
)

// checkCookie looks at the request headers to verify a connect.sid cookie is present
// before handing over to the next chain of HandlerFunc(s)
func (p *PopSocket) checkCookie(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := checkSessionCookiePresence(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		sid, err := extractSessionID(cookie)
		if err != nil {
			http.Error(w, unauthorized_session_err, http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx, err = p.SessionStore.Find(ctx, sid)
		if err != nil {
			http.Error(w, unauthorized_session_err, http.StatusUnauthorized)
			return
		}

		next(w, r.WithContext(ctx))
	}
}

func checkSessionCookiePresence(r *http.Request) (string, error) {
	cookie, err := r.Cookie("connect.sid")
	if err != nil || cookie.Value == "" {
		return "", fmt.Errorf(unauthorized_session_err)
	}

	return cookie.Value, nil
}

func extractSessionID(cookie string) (string, error) {
	sid, err := util.DecodeCookie(cookie, config.ENV.SESSION_SECRET_KEY.Value)
	if err != nil {
		return "", fmt.Errorf(unauthorized_session_err)
	}

	return sid, nil
}
