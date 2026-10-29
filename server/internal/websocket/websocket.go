package websocket

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/db"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/arumes31/patchli/server/internal/proxy"
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

const (
	// maxMessageBytes caps a single agent message.
	maxMessageBytes = 64 << 10
	// readTimeout must exceed the agent heartbeat interval (30s).
	readTimeout = 2 * time.Minute
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// X-Forwarded-Proto is only honored from TRUSTED_PROXY.
		expectedOrigin := proxy.Scheme(r) + "://" + r.Host
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
	}, jwt.WithAudience(auth.TokenAudience), jwt.WithExpirationRequired())

	if err != nil || !token.Valid {
		http.Error(w, "Unauthorized - Invalid Token", http.StatusUnauthorized)
		return
	}

	// Only access tokens are accepted, and the node identity is bound to the
	// token subject for the lifetime of the connection.
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["typ"] != auth.TokenTypeAccess {
		http.Error(w, "Unauthorized - Invalid Token", http.StatusUnauthorized)
		return
	}
	nodeID, err := claims.GetSubject()
	if err != nil || nodeID == "" {
		http.Error(w, "Unauthorized - Invalid Token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	conn.SetReadLimit(maxMessageBytes)
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readTimeout))
	})

	var registered bool

	// markOffline only cleans up if this connection still owns the node ID;
	// a newer connection for the same node may have replaced it.
	markOffline := func() {
		if !registered || !fleet.Registry.UnregisterConn(nodeID, conn) {
			return
		}
		log.Printf("Agent %q disconnected", nodeID) // #nosec G706 -- %q escapes control characters.
		if err := db.MarkNodeOffline(nodeID); err != nil {
			log.Printf("Failed to update node status for %q on disconnect: %v", nodeID, err) // #nosec G706 -- %q escapes control characters.
		}
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			markOffline()
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

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
			if p.MacAddress != "" && p.MacAddress != nodeID {
				log.Printf("Rejecting heartbeat: node ID %q does not match token subject %q", p.MacAddress, nodeID) // #nosec G706 -- %q escapes control characters.
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "node ID mismatch"),
					time.Now().Add(time.Second))
				markOffline()
				return
			}
			p.MacAddress = nodeID
			fleet.Registry.Register(nodeID, conn, p)
			registered = true

			if err := db.UpdateNodeStatus(nodeID, p.Hostname, p.OS, p.OSVersion, p.Kernel, "online"); err != nil {
				log.Printf("Failed to update node status for %q: %v", nodeID, err) // #nosec G706 -- %q escapes control characters.
			}

		case "log":
			// Handle log payload
		}
	}
}
