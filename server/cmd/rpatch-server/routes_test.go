package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arumes31/rpatch/server/internal/auth"
)

// TestRoutes checks that pages and assets are served and that every admin
// route rejects unauthenticated requests.
func TestRoutes(t *testing.T) {
	auth.SetTestSecrets()
	t.Setenv("ADMIN_TOKEN", "secret-admin")
	mux := newMux()
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/health", http.StatusOK},
		{"GET", "/login", http.StatusOK},
		{"GET", "/assets/css/app.css", http.StatusOK},
		{"GET", "/favicon.png", http.StatusOK},
		{"GET", "/", http.StatusFound},
		{"GET", "/setup", http.StatusFound},
		{"GET", "/nope", http.StatusNotFound},
		{"GET", "/download/agent.manifest", http.StatusNotFound},
		{"GET", "/api/v1/nodes", http.StatusUnauthorized},
		{"GET", "/api/v1/stats", http.StatusUnauthorized},
		{"GET", "/api/v1/setup", http.StatusUnauthorized},
		{"POST", "/api/v1/nodes/revoke", http.StatusUnauthorized},
		{"POST", "/api/v1/nodes/n1/action", http.StatusUnauthorized},
		{"PUT", "/api/v1/nodes/n1/policy", http.StatusUnauthorized},
		{"GET", "/api/v1/jobs", http.StatusUnauthorized},
		{"GET", "/api/v1/jobs/j1", http.StatusUnauthorized},
		{"GET", "/api/v1/settings", http.StatusUnauthorized},
		{"PUT", "/api/v1/settings", http.StatusUnauthorized},
		{"GET", "/api/v1/window", http.StatusUnauthorized},
		{"POST", "/api/v1/window/run", http.StatusUnauthorized},
		{"GET", "/ws", http.StatusUnauthorized},
	} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
		if rr.Code != tc.want {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, rr.Code, tc.want)
		}
	}
}
