package websocket

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/arumes31/patchli/server/internal/db"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret != "" && len(secret) >= 16 {
		jwtSecret = []byte(secret)
	}
}

// InitSecrets loads the JWT secret from the environment. Called by the
// server main at startup; panics if the secret is missing or too short.
func InitSecrets() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" || len(secret) < 16 {
		panic("JWT_SECRET environment variable is required and must be at least 16 characters")
	}
	jwtSecret = []byte(secret)
}

// SetTestSecrets sets a dummy JWT secret for unit tests.
func SetTestSecrets() {
	jwtSecret = []byte("test-jwt-secret-that-is-long-enough")
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		expectedOrigin := "http://" + r.Host
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			if strings.ToLower(proto) == "https" {
				expectedOrigin = "https://" + r.Host
			}
		} else if r.TLS != nil {
			expectedOrigin = "https://" + r.Host
		}
		if origin != "" && origin != expectedOrigin {
			return false
		}
		return true
	},
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Require Auth
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "Unauthorized - Invalid Header", http.StatusUnauthorized)
		return
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		http.Error(w, "Unauthorized - Invalid Token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	var macAddr string
	var lastOS string

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if macAddr != "" {
				fleet.Registry.Unregister(macAddr)
				log.Printf("Agent %s disconnected", macAddr)
				if err := db.UpdateNodeStatus(macAddr, "", "", lastOS, "", "offline"); err != nil {
					log.Printf("Failed to update node status for %s on disconnect: %v", macAddr, err)
				}
			}
			break
		}

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
			macAddr = p.MacAddress
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
