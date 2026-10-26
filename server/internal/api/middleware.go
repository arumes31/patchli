package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
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
		if subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// contentSecurityPolicy restricts where pages may load from and send data to.
// Inline scripts are still allowed because the embedded pages use inline
// event handlers; dynamic content must therefore never be inserted as HTML.
const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'"

// SecurityHeaders adds browser hardening headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// RateLimiter is a fixed-window per-client-IP request limiter.
type RateLimiter struct {
	limit  int
	window time.Duration

	mu      sync.Mutex
	clients map[string]*rateWindow
}

type rateWindow struct {
	start time.Time
	count int
}

// NewRateLimiter allows limit requests per client IP within each window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, clients: make(map[string]*rateWindow)}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	// Drop expired windows so the map cannot grow without bound.
	if len(rl.clients) > 10000 {
		for k, v := range rl.clients {
			if now.Sub(v.start) >= rl.window {
				delete(rl.clients, k)
			}
		}
	}
	w, ok := rl.clients[ip]
	if !ok || now.Sub(w.start) >= rl.window {
		rl.clients[ip] = &rateWindow{start: now, count: 1}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}

// Middleware rejects requests above the limit with 429.
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}
