package api

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/arumes31/rpatch/server/internal/proxy"
)

// AuthMiddleware protects admin endpoints. It accepts a Bearer token matching
// ADMIN_TOKEN (scripts) or an admin session cookie (browser UI). Cookie
// requests that change state must come from the same origin.
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			if os.Getenv("ADMIN_TOKEN") == "" {
				http.Error(w, "Unauthorized - Server Configuration Error", http.StatusUnauthorized)
				return
			}
			if !validAdminToken(strings.TrimPrefix(authHeader, "Bearer ")) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if !hasAdminSession(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
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
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// clientIP keys rate limits per client. Behind TRUSTED_PROXY every request
// arrives from the proxy, so the first X-Forwarded-For entry is used there.
// shortcut: that entry is client-controlled, so a client can dodge its own
// limit but cannot exhaust another client's; fine while ADMIN_TOKEN is high-entropy.
func clientIP(r *http.Request) string {
	if proxy.IsTrusted(r.RemoteAddr) {
		if first, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ","); strings.TrimSpace(first) != "" {
			return strings.TrimSpace(first)
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
