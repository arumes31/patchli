package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/models"
)

func init() {
	// Set environment variables for tests
	os.Setenv("REGISTRATION_SECRET", "testregsecret1234")
	os.Setenv("JWT_SECRET", "atleast16charslongsecret")
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
		defer ws.Close()

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
			defer ws.Close()
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
			ws.Close()
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
