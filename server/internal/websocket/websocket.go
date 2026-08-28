package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/db"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/gorilla/websocket"
)

// SetTestSecrets sets a dummy JWT secret for unit tests.
func SetTestSecrets() {
	auth.SetTestSecrets()
}

func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Native agents do not send browser Origin headers.
	}
	publicOrigin, err := url.Parse(strings.TrimSpace(os.Getenv("BASE_URL")))
	if err != nil || publicOrigin.Scheme != "https" || publicOrigin.Host == "" {
		return false
	}
	parsedOrigin, err := url.Parse(origin)
	return err == nil && parsedOrigin.Scheme == publicOrigin.Scheme && parsedOrigin.Host == publicOrigin.Host && parsedOrigin.Path == ""
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Require Auth
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "Unauthorized - Invalid Header", http.StatusUnauthorized)
		return
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	claims, err := auth.ValidateAccessToken(tokenStr)
	if err != nil {
		http.Error(w, "Unauthorized - Invalid Token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(64 << 10)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})

	var macAddr string
	var lastOS string
	defer func() {
		if macAddr == "" {
			return
		}
		fleet.Registry.Unregister(macAddr)
		// #nosec G706 -- the signed JWT subject is registration-validated and %q escapes control characters.
		log.Printf("Agent %q disconnected", macAddr)
		if err := db.UpdateNodeStatus(macAddr, "", "", lastOS, "", "offline"); err != nil {
			// #nosec G706 -- the signed JWT subject is registration-validated and %q escapes control characters.
			log.Printf("Failed to update node status for %q on disconnect: %v", macAddr, err)
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		var wsMsg struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			log.Printf("Unmarshal error: %v", err)
			continue
		}

		switch wsMsg.Type {
		case "heartbeat":
			var p models.HeartbeatPayload
			if err := json.Unmarshal(wsMsg.Payload, &p); err != nil {
				log.Printf("Heartbeat unmarshal error: %v", err)
				continue
			}
			if p.MacAddress != claims.Subject {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "agent identity mismatch"), time.Now().Add(time.Second))
				return
			}
			macAddr = claims.Subject
			lastOS = p.OSVersion
			fleet.Registry.Register(macAddr, conn, p)

			if err := db.UpdateNodeStatus(p.MacAddress, p.Hostname, p.OS, p.OSVersion, p.Kernel, "online"); err != nil {
				log.Printf("Failed to update node status for %s: %v", p.MacAddress, err)
			}

		case "log":
			// Handle log payload
		}
	}
}
