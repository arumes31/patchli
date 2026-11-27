package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arumes31/rpatch/server/internal/auth"
)

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func loginCookie(t *testing.T) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://rpatch.test/api/v1/admin/login", strings.NewReader(`{"token":"secret-admin"}`))
	req.Header.Set("Origin", "http://rpatch.test")
	rr := httptest.NewRecorder()
	HandleAdminLogin(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("login: got %d", rr.Code)
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == SessionCookie {
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("cookie flags: %+v", c)
			}
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func TestAdminLogin(t *testing.T) {
	auth.SetTestSecrets()
	t.Setenv("ADMIN_TOKEN", "secret-admin")
	loginCookie(t)

	for name, tc := range map[string]struct{ body, origin string }{
		"wrong token":  {`{"token":"nope"}`, "http://rpatch.test"},
		"cross origin": {`{"token":"secret-admin"}`, "http://evil.test"},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://rpatch.test/api/v1/admin/login", strings.NewReader(tc.body))
		req.Header.Set("Origin", tc.origin)
		rr := httptest.NewRecorder()
		HandleAdminLogin(rr, req)
		if rr.Code == http.StatusNoContent || len(rr.Result().Cookies()) != 0 {
			t.Errorf("%s: got %d", name, rr.Code)
		}
	}

	t.Setenv("ADMIN_TOKEN", "")
	req := httptest.NewRequest(http.MethodPost, "http://rpatch.test/api/v1/admin/login", strings.NewReader(`{"token":""}`))
	rr := httptest.NewRecorder()
	HandleAdminLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("empty ADMIN_TOKEN must never log in, got %d", rr.Code)
	}
}

func TestAuthMiddlewareSession(t *testing.T) {
	auth.SetTestSecrets()
	t.Setenv("ADMIN_TOKEN", "secret-admin")
	cookie := loginCookie(t)
	agentToken, _ := auth.GenerateAgentJWT("aa:bb")

	cases := []struct {
		name   string
		method string
		cookie *http.Cookie
		origin string
		want   int
	}{
		{"session GET", http.MethodGet, cookie, "", http.StatusOK},
		{"session POST same origin", http.MethodPost, cookie, "http://rpatch.test", http.StatusOK},
		{"session POST cross origin", http.MethodPost, cookie, "http://evil.test", http.StatusForbidden},
		{"no session", http.MethodGet, nil, "", http.StatusUnauthorized},
		{"agent token as cookie", http.MethodGet, &http.Cookie{Name: SessionCookie, Value: agentToken}, "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, "http://rpatch.test/api/v1/nodes", nil)
		if tc.cookie != nil {
			req.AddCookie(tc.cookie)
		}
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		rr := httptest.NewRecorder()
		AuthMiddleware(okHandler)(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.name, rr.Code, tc.want)
		}
	}
}

func TestAdminSessionIsNotAnAgentToken(t *testing.T) {
	auth.SetTestSecrets()
	s, err := auth.NewAdminSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ValidateAccessToken(s); err == nil {
		t.Error("admin session accepted as agent access token")
	}
	if _, err := auth.ValidateRefreshToken(s); err == nil {
		t.Error("admin session accepted as agent refresh token")
	}
}

func TestRequireAdminPage(t *testing.T) {
	auth.SetTestSecrets()
	t.Setenv("ADMIN_TOKEN", "secret-admin")
	rr := httptest.NewRecorder()
	RequireAdminPage(okHandler)(rr, httptest.NewRequest(http.MethodGet, "http://rpatch.test/", nil))
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/login" {
		t.Errorf("no session: got %d %q", rr.Code, rr.Header().Get("Location"))
	}
	req := httptest.NewRequest(http.MethodGet, "http://rpatch.test/", nil)
	req.AddCookie(loginCookie(t))
	rr = httptest.NewRecorder()
	RequireAdminPage(okHandler)(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("with session: got %d", rr.Code)
	}
}

func TestAdminLoginRateLimit(t *testing.T) {
	auth.SetTestSecrets()
	t.Setenv("ADMIN_TOKEN", "secret-admin")
	t.Setenv("TRUSTED_PROXY", "10.0.0.1")
	h := NewRateLimiter(5, time.Minute).Middleware(HandleAdminLogin)
	try := func(fwd string) int {
		req := httptest.NewRequest(http.MethodPost, "http://rpatch.test/api/v1/admin/login", strings.NewReader(`{"token":"x"}`))
		req.RemoteAddr = "10.0.0.1:5555"
		req.Header.Set("X-Forwarded-For", fwd)
		req.Header.Set("Origin", "http://rpatch.test")
		rr := httptest.NewRecorder()
		h(rr, req)
		return rr.Code
	}
	for i := 0; i < 5; i++ {
		if c := try("1.1.1.1"); c != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d", i+1, c)
		}
	}
	if c := try("1.1.1.1, 10.0.0.1"); c != http.StatusTooManyRequests {
		t.Errorf("6th attempt: got %d, want 429", c)
	}
	if c := try("2.2.2.2"); c != http.StatusUnauthorized {
		t.Errorf("another client behind the proxy must not be limited, got %d", c)
	}
}
