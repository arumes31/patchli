package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/patchli/server/internal/db"
	"github.com/patchli/server/internal/fleet"
	"github.com/patchli/server/internal/models"
)

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
	if authHeader == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
