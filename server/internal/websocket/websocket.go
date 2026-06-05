package websocket

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/arumes31/patchli/server/internal/db"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
)

var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
		if len(secret) < 16 {
			secret = "atleast16charslongsecret"
		}
	} else if secret == "" || len(secret) < 16 {
		log.Fatal("JWT_SECRET environment variable is required and must be at least 16 characters")
	}
	jwtSecret = []byte(secret)
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
				_ = db.UpdateNodeStatus(macAddr, "", "", lastOS, "", "offline")
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

			_ = db.UpdateNodeStatus(p.MacAddress, p.Hostname, p.OS, p.OSVersion, p.Kernel, "online")

		case "log":
			// Handle log payload
		}
		}
		}
