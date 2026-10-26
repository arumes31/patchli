package websocket

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func init() {
	auth.SetTestSecrets()
	SetTestSecrets()
}

func TestHandleWebSocket(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(HandleWebSocket))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")

	token, _ := auth.GenerateAgentJWT("test-mac")
	header := http.Header{}
	header.Add("Authorization", "Bearer "+token)

	t.Run("Heartbeat and Logs", func(t *testing.T) {
		ws, _, err := websocket.DefaultDialer.Dial(u, header)
		if err != nil {
			t.Fatalf("Failed to connect to websocket: %v", err)
		}
		defer func() { _ = ws.Close() }()

		heartbeat := struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}{
			Type:    "heartbeat",
			Payload: json.RawMessage(`{"mac_address":"test-mac", "hostname":"test-node", "os":"Linux"}`),
		}
		_ = ws.WriteJSON(heartbeat)

		logMsg := struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}{
			Type:    "log",
			Payload: json.RawMessage(`{"job_id":"123", "data":"test log"}`),
		}
		_ = ws.WriteJSON(logMsg)

		time.Sleep(50 * time.Millisecond)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		ws, _, _ := websocket.DefaultDialer.Dial(u, header)
		if ws != nil {
			defer func() { _ = ws.Close() }()
			_ = ws.WriteMessage(websocket.TextMessage, []byte("invalid json"))
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("Disconnect", func(t *testing.T) {
		ws, _, _ := websocket.DefaultDialer.Dial(u, header)
		if ws != nil {
			heartbeat := struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}{
				Type:    "heartbeat",
				Payload: json.RawMessage(`{"mac_address":"test-mac", "hostname":"temp"}`),
			}
			_ = ws.WriteJSON(heartbeat)
			time.Sleep(50 * time.Millisecond)
			if err := ws.Close(); err != nil {
				t.Fatal(err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
}

func FuzzMessage(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"type":"heartbeat","payload":{"mac_address":"00:11:22:33:44:55","hostname":"test-node","os":"Linux"}}`),
		[]byte(`{"type":"log","payload":{"job_id":"123","data":"test log"}}`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var wsMsg struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(data, &wsMsg); err != nil {
			return
		}

		switch wsMsg.Type {
		case "heartbeat":
			var p models.HeartbeatPayload
			_ = json.Unmarshal(wsMsg.Payload, &p)
		case "log":
			// ...
		}
	})
}

func TestInitSecrets(t *testing.T) {
	original := append([]byte(nil), jwtSecret...)
	t.Cleanup(func() { jwtSecret = original })
	for _, tc := range []struct {
		name, secret  string
		panicExpected bool
	}{
		{"missing", "", true}, {"short", "short", true}, {"valid", "0123456789abcdef", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", tc.secret)
			defer func() {
				panicked := recover() != nil
				if panicked != tc.panicExpected {
					t.Errorf("panicked=%v want=%v", panicked, tc.panicExpected)
				}
			}()
			InitSecrets()
			if string(jwtSecret) != tc.secret {
				t.Error("configured secret not loaded")
			}
		})
	}
}

func TestOriginPolicy(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "192.0.2.1")
	for _, tc := range []struct {
		name, origin, proto, remote string
		tls, want                   bool
	}{
		{name: "agent without origin", want: true},
		{name: "same host", origin: "http://controller.example", want: true},
		{name: "foreign host", origin: "http://attacker.example", want: false},
		{name: "direct TLS", origin: "https://controller.example", tls: true, want: true},
		{name: "TLS scheme mismatch", origin: "http://controller.example", tls: true, want: false},
		{name: "forwarded TLS from trusted proxy", origin: "https://controller.example", proto: "HTTPS", remote: "192.0.2.1:1234", want: true},
		{name: "forwarded TLS from untrusted client", origin: "https://controller.example", proto: "HTTPS", remote: "192.0.2.2:1234", want: false},
		{name: "unsupported forwarded scheme", origin: "https://controller.example", proto: "ftp", remote: "192.0.2.1:1234", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://controller.example/ws", nil)
			if tc.remote != "" {
				req.RemoteAddr = tc.remote
			}
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("X-Forwarded-Proto", tc.proto)
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if got := upgrader.CheckOrigin(req); got != tc.want {
				t.Errorf("accepted=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestHandleWebSocketAuthentication(t *testing.T) {
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "node"}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := auth.GenerateAgentJWT("node")
	if err != nil {
		t.Fatal(err)
	}
	pair, err := auth.GenerateTokenPair("node")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "node", "exp": time.Now().Add(time.Hour).Unix()}).SignedString(jwtSecret)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, header string
		status       int
		message      string
	}{
		{"missing header", "", http.StatusUnauthorized, "Invalid Header"},
		{"wrong scheme", "Basic token", http.StatusUnauthorized, "Invalid Header"},
		{"malformed token", "Bearer invalid", http.StatusUnauthorized, "Invalid Token"},
		{"unsigned claims", "Bearer " + unsigned, http.StatusUnauthorized, "Invalid Token"},
		{"refresh token", "Bearer " + pair.RefreshToken, http.StatusUnauthorized, "Invalid Token"},
		{"untyped token", "Bearer " + legacy, http.StatusUnauthorized, "Invalid Token"},
		{"missing websocket handshake", "Bearer " + valid, http.StatusBadRequest, "Bad Request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://controller.example/ws", nil)
			req.Header.Set("Authorization", tc.header)
			rr := httptest.NewRecorder()
			HandleWebSocket(rr, req)
			if rr.Code != tc.status || !strings.Contains(strings.ToLower(rr.Body.String()), strings.ToLower(tc.message)) {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleWebSocketIgnoresMalformedHeartbeat(t *testing.T) {
	before := len(fleet.Registry.GetNodes())
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(done); HandleWebSocket(w, r) }))
	defer server.Close()
	token, err := auth.GenerateAgentJWT("node")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), header)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"heartbeat","payload":"wrong shape"}`)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		if got := len(fleet.Registry.GetNodes()); got != before {
			t.Fatalf("malformed heartbeat changed node count: got %d want %d", got, before)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not process malformed message and close")
	}
}

func TestHandleWebSocketRejectsForeignNodeID(t *testing.T) {
	fleet.Registry.Reset()
	t.Cleanup(fleet.Registry.Reset)
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(done); HandleWebSocket(w, r) }))
	defer server.Close()
	token, err := auth.GenerateAgentJWT("attacker-node")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), header)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"heartbeat","payload":{"mac_address":"victim-node","hostname":"victim"}}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not close the impersonating connection")
	}
	for _, n := range fleet.Registry.GetNodes() {
		if n.MacAddress == "victim-node" {
			t.Fatal("impersonated node was registered")
		}
	}
}

func TestHandleWebSocketBindsNodeToTokenSubject(t *testing.T) {
	fleet.Registry.Reset()
	t.Cleanup(fleet.Registry.Reset)
	server := httptest.NewServer(http.HandlerFunc(HandleWebSocket))
	defer server.Close()
	token, err := auth.GenerateAgentJWT("bound-node")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), header)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// Two heartbeats on the same connection must keep it registered.
	for i := 0; i < 2; i++ {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"heartbeat","payload":{"hostname":"bound"}}`)); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		nodes := fleet.Registry.GetNodes()
		if len(nodes) == 1 && nodes[0].MacAddress == "bound-node" && nodes[0].Status == "Online" {
			if err := fleet.Registry.SendCommand("bound-node", models.CommandPayload{ID: "job", Action: "check_updates"}); err != nil {
				t.Fatalf("connection was not kept registered: %v", err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("node not registered under token subject: %+v", fleet.Registry.GetNodes())
}
