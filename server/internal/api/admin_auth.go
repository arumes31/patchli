package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/url"
	"os"

	"github.com/arumes31/rpatch/server/internal/auth"
)

// SessionCookie holds the admin UI session.
const SessionCookie = "rpatch_session"

func validAdminToken(token string) bool {
	adminToken := os.Getenv("ADMIN_TOKEN")
	return adminToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) == 1
}

func hasAdminSession(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie)
	// Unsetting ADMIN_TOKEN locks the UI, including existing sessions.
	return err == nil && os.Getenv("ADMIN_TOKEN") != "" && auth.ValidAdminSession(c.Value)
}

// sameOrigin rejects cross-site requests that ride on the session cookie.
// Browsers always send Origin on cross-origin and on same-origin POSTs.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true, // browsers also accept Secure cookies on http://localhost
		SameSite: http.SameSiteStrictMode,
	})
}

// HandleAdminLogin exchanges ADMIN_TOKEN for a browser session cookie.
func HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if !validAdminToken(req.Token) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	session, err := auth.NewAdminSession()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, session, int(auth.AdminSessionDuration.Seconds()))
	w.WriteHeader(http.StatusNoContent)
}

// HandleAdminLogout clears the session cookie.
func HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	// shortcut: the session JWT stays valid until expiry (12h) if copied; add a server-side revocation list if sessions must die on logout.
	setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// RequireAdminPage redirects browsers without a session to /login.
func RequireAdminPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasAdminSession(r) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	}
}
