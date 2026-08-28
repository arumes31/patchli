package control

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestFromEnvironmentRejectsPlaintextAndInvalidOrigins(t *testing.T) {
	for _, raw := range []string{"", "http://server.example", "https://user:pass@server.example", "https://server.example/path"} {
		if _, err := FromEnvironment(env(map[string]string{"SERVER_URL": raw})); err == nil {
			t.Fatalf("accepted insecure SERVER_URL %q", raw)
		}
	}
}

func TestClientVerifiesTLSAndSupportsPrivateCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := FromEnvironment(env(map[string]string{"SERVER_URL": server.URL}))
	if err != nil {
		t.Fatal(err)
	}
	request, err := client.NewRequest(context.Background(), http.MethodGet, "/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(request); err == nil {
		t.Fatal("client accepted an untrusted certificate")
	}

	directory := t.TempDir()
	caFile := filepath.Join(directory, "ca.pem")
	certificate := server.Certificate()
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(caFile, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	client, err = FromEnvironment(env(map[string]string{"SERVER_URL": server.URL, "SERVER_CA_FILE": caFile}))
	if err != nil {
		t.Fatal(err)
	}
	request, _ = client.NewRequest(context.Background(), http.MethodGet, "/health", nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("private CA request failed: %v", err)
	}
	_ = response.Body.Close()

	escaping, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://evil.example/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(escaping); err == nil {
		t.Fatal("client accepted a request outside the configured origin")
	}
}
