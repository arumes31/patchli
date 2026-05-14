package websocket

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/patchli/server/internal/db"
	"github.com/patchli/server/internal/fleet"
	"github.com/patchli/server/internal/models"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	var macAddr string

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if macAddr != "" {
				fleet.Registry.Unregister(macAddr)
				log.Printf("Agent %s disconnected", macAddr)
				_ = db.UpdateNodeStatus(macAddr, "", "", "", "", "Offline")
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
			fleet.Registry.Register(macAddr, conn, p)
			
			_ = db.UpdateNodeStatus(p.MacAddress, p.Hostname, p.OS, "", p.Kernel, "Online")

		case "log":
			// Handle log payload
		}
	}
}
