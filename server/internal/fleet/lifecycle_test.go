package fleet

import (
	"errors"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func socketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		accepted <- conn
	}))
	t.Cleanup(server.Close)
	client, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	var peer *websocket.Conn
	select {
	case peer = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("upgrade timed out")
	}
	t.Cleanup(func() { _ = client.Close(); _ = peer.Close() })
	return peer, client
}

func TestAgentLifecycleAndCommandDelivery(t *testing.T) {
	manager := &AgentManager{}
	manager.Reset()
	peer, client := socketPair(t)
	manager.Register("node", peer, models.HeartbeatPayload{MacAddress: "node", Hostname: "host", OS: "linux", Kernel: "6"})
	nodes := manager.GetNodes()
	if len(nodes) != 1 || nodes[0].Hostname != "host" || nodes[0].Status != "Online" {
		t.Fatalf("registration missing: %+v", nodes)
	}
	nodes[0].Hostname = "mutated"
	if manager.GetNodes()[0].Hostname != "host" {
		t.Fatal("GetNodes exposed internal state")
	}
	manager.SetStatus("node", "patching")
	manager.SetStatus("unknown", "offline")
	if manager.GetNodes()[0].Status != "patching" {
		t.Fatal("status not updated")
	}
	if err := manager.SendCommand("node", models.CommandPayload{ID: "job", Action: "apply_updates", Packages: []string{"curl"}}); err != nil {
		t.Fatal(err)
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var message struct {
		Type    string                `json:"type"`
		Payload models.CommandPayload `json:"payload"`
	}
	if err := client.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "command" || message.Payload.ID != "job" || message.Payload.Packages[0] != "curl" {
		t.Fatalf("wrong command: %+v", message)
	}
	replacement, _ := socketPair(t)
	manager.Register("node", replacement, models.HeartbeatPayload{MacAddress: "node", Hostname: "replacement"})
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.ReadMessage(); err == nil {
		t.Fatal("re-registration left previous connection open")
	} else {
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			t.Fatal("old connection timed out instead of being closed")
		}
	}
	if !manager.AcquireJob("node", "job") {
		t.Fatal("could not acquire job")
	}
	if manager.AcquireJob("node", "conflicting-job") {
		t.Fatal("accepted concurrent jobs for the same node")
	}
	manager.ReleaseJob("node")
	if !manager.AcquireJob("node", "next-job") {
		t.Fatal("released node remained busy")
	}
	manager.Unregister("node")
	if manager.GetNodes()[0].Status != "offline" {
		t.Fatal("unregister did not mark offline")
	}
	if err := manager.SendCommand("node", models.CommandPayload{}); !errors.Is(err, models.ErrAgentOffline) {
		t.Fatalf("offline command: %v", err)
	}
	if !manager.AcquireJob("node", "replacement-job") {
		t.Fatal("unregister leaked active job")
	}
	manager.Unregister("unknown")
}

func TestPruneRemovesOnlyStaleAndInvalidHeartbeats(t *testing.T) {
	manager := &AgentManager{}
	manager.Reset()
	for id, stamp := range map[string]string{"fresh": time.Now().Format(time.RFC3339), "old": time.Now().Add(-time.Hour).Format(time.RFC3339), "invalid": "bad"} {
		manager.details[id] = &models.AgentDetails{MacAddress: id, LastHeartbeat: stamp}
		manager.activeJobs[id] = "job"
	}
	manager.PruneStaleAgents(time.Minute)
	nodes := manager.GetNodes()
	if len(nodes) != 1 || nodes[0].MacAddress != "fresh" {
		t.Fatalf("incorrect remaining nodes: %+v", nodes)
	}
	if !manager.AcquireJob("old", "new") || !manager.AcquireJob("invalid", "new") {
		t.Fatal("pruning leaked active jobs")
	}
	manager.Reset()
	if len(manager.GetNodes()) != 0 {
		t.Fatal("reset did not clear nodes")
	}
}
