package models

import (
	"encoding/json"
	"testing"
)

func TestAgentDetailsJSON(t *testing.T) {
	agent := AgentDetails{
		Hostname:      "test-host",
		MacAddress:    "00:11:22:33:44:55",
		OS:            "linux",
		Kernel:        "5.15.0",
		Status:        "online",
		LastHeartbeat: "2023-10-27T10:00:00Z",
	}

	data, err := json.Marshal(agent)
	if err != nil {
		t.Fatalf("Failed to marshal AgentDetails: %v", err)
	}

	var unmarshaled AgentDetails
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal AgentDetails: %v", err)
	}

	if unmarshaled.Hostname != agent.Hostname {
		t.Errorf("Expected Hostname %s, got %s", agent.Hostname, unmarshaled.Hostname)
	}
	if unmarshaled.MacAddress != agent.MacAddress {
		t.Errorf("Expected MacAddress %s, got %s", agent.MacAddress, unmarshaled.MacAddress)
	}
}

func TestHeartbeatPayloadJSON(t *testing.T) {
	payload := HeartbeatPayload{
		MacAddress:   "00:11:22:33:44:55",
		Hostname:     "test-host",
		OS:           "linux",
		OSVersion:    "Ubuntu 22.04",
		Kernel:       "5.15.0",
		RebootNeeded: true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal HeartbeatPayload: %v", err)
	}

	var unmarshaled HeartbeatPayload
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal HeartbeatPayload: %v", err)
	}

	if unmarshaled.RebootNeeded != payload.RebootNeeded {
		t.Errorf("Expected RebootNeeded %v, got %v", payload.RebootNeeded, unmarshaled.RebootNeeded)
	}
}

func TestCommandPayloadJSON(t *testing.T) {
	payload := CommandPayload{
		ID:       "cmd-123",
		Action:   "patch",
		Packages: []string{"vim", "curl"},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal CommandPayload: %v", err)
	}

	var unmarshaled CommandPayload
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal CommandPayload: %v", err)
	}

	if len(unmarshaled.Packages) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(unmarshaled.Packages))
	}

	// Test omitempty
	payloadEmpty := CommandPayload{
		ID:     "cmd-456",
		Action: "reboot",
	}
	dataEmpty, _ := json.Marshal(payloadEmpty)
	var mapEmpty map[string]interface{}
	json.Unmarshal(dataEmpty, &mapEmpty)

	if _, ok := mapEmpty["packages"]; ok {
		t.Error("packages should be omitted when empty")
	}
}
