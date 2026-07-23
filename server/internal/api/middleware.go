package api

import (
	"net/http"
	"os"
	"strings"
)

// AuthMiddleware protects endpoints by requiring a Bearer token matching ADMIN_TOKEN.
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminToken := os.Getenv("ADMIN_TOKEN")

		// If ADMIN_TOKEN is not set, we default to unauthorized for safety.
		if adminToken == "" {
			http.Error(w, "Unauthorized - Server Configuration Error", http.StatusUnauthorized)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != adminToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
}
