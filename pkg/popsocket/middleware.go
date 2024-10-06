package popsocket

import (
	"fmt"
	"net/http"
)

// checkSession looks at the request headers to verify a connect.sid cookie is present
// before handing over to the next chain of HandlerFunc(s)
func checkSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("connect.sid")
		if err != nil || cookie.Value == "" {
			http.Error(w, "Unauthorized: Missing or invalid session", http.StatusUnauthorized)
			Logger().Info(fmt.Sprintf("[CheckSession]: %v", err))
			return
		}
		next(w, r)
	}
}
