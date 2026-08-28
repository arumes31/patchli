package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/models"
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
			Payload: json.RawMessage(`{"mac_address":"00:11:22:33:44:55", "hostname":"test-node", "os":"Linux"}`),
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
				Payload: json.RawMessage(`{"mac_address":"disconnect-mac", "hostname":"temp"}`),
			}
			_ = ws.WriteJSON(heartbeat)
			time.Sleep(50 * time.Millisecond)
			_ = ws.Close()
			time.Sleep(50 * time.Millisecond)
		}
	})
}

func TestHandleWebSocketBindsHeartbeatToTokenSubject(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(HandleWebSocket))
	defer s.Close()

	token, err := auth.GenerateAgentJWT("expected-agent")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"Authorization": {"Bearer " + token}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), header)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	heartbeat := map[string]any{
		"type":    "heartbeat",
		"payload": map[string]any{"mac_address": "impersonated-agent", "hostname": "attacker"},
	}
	if err := conn.WriteJSON(heartbeat); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("server kept an identity-mismatched WebSocket open")
	}
}

func TestHandleWebSocketRejectsRefreshTokenAndCrossOriginBrowser(t *testing.T) {
	t.Setenv("BASE_URL", "https://patchli.example.test")
	s := httptest.NewServer(http.HandlerFunc(HandleWebSocket))
	defer s.Close()

	pair, err := auth.GenerateTokenPair("agent")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"Authorization": {"Bearer " + pair.RefreshToken}}
	if conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), header); err == nil {
		_ = conn.Close()
		t.Fatal("refresh token was accepted for WebSocket access")
	} else if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh token status = %v, error = %v", response, err)
	}

	accessHeader := http.Header{
		"Authorization": {"Bearer " + pair.AccessToken},
		"Origin":        {"https://evil.example"},
	}
	if conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), accessHeader); err == nil {
		_ = conn.Close()
		t.Fatal("cross-origin browser WebSocket was accepted")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin status = %v, error = %v", response, err)
	}
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
