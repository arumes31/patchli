package proxy

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScheme(t *testing.T) {
	for _, tc := range []struct {
		name, trusted, remote, proto string
		tls                          bool
		want                         string
	}{
		{name: "plain", remote: "192.0.2.2:1234", want: "http"},
		{name: "direct TLS", remote: "192.0.2.2:1234", tls: true, want: "https"},
		{name: "untrusted forwarded header", trusted: "192.0.2.1", remote: "192.0.2.2:1234", proto: "https", want: "http"},
		{name: "trusted forwarded header", trusted: "192.0.2.1", remote: "192.0.2.1:1234", proto: "HTTPS", want: "https"},
		{name: "trusted forwarded http over TLS", trusted: "192.0.2.1", remote: "192.0.2.1:1234", proto: "http", tls: true, want: "http"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXY", tc.trusted)
			req := httptest.NewRequest(http.MethodGet, "http://controller.example/", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-Proto", tc.proto)
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if got := Scheme(req); got != tc.want {
				t.Fatalf("Scheme()=%q want %q", got, tc.want)
			}
		})
	}
}
