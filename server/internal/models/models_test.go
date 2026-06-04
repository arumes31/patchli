package models

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAgentDetails_JSON(t *testing.T) {
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
		t.Fatalf("failed to marshal AgentDetails: %v", err)
	}

	var unmarshaled AgentDetails
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal AgentDetails: %v", err)
	}

	if !reflect.DeepEqual(agent, unmarshaled) {
		t.Errorf("AgentDetails mismatch after JSON roundtrip.\nGot: %+v\nWant: %+v", unmarshaled, agent)
	}

	// Verify specific tags
	expectedJSON := `{"hostname":"test-host","mac_address":"00:11:22:33:44:55","os":"linux","kernel":"5.15.0","status":"online","last_heartbeat":"2023-10-27T10:00:00Z"}`
	if string(data) != expectedJSON {
		t.Errorf("JSON output mismatch.\nGot: %s\nWant: %s", string(data), expectedJSON)
	}
}

func TestHeartbeatPayload_JSON(t *testing.T) {
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
		t.Fatalf("failed to marshal HeartbeatPayload: %v", err)
	}

	var unmarshaled HeartbeatPayload
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal HeartbeatPayload: %v", err)
	}

	if !reflect.DeepEqual(payload, unmarshaled) {
		t.Errorf("HeartbeatPayload mismatch after JSON roundtrip.\nGot: %+v\nWant: %+v", unmarshaled, payload)
	}

	expectedJSON := `{"mac_address":"00:11:22:33:44:55","hostname":"test-host","os":"linux","os_version":"Ubuntu 22.04","kernel":"5.15.0","reboot_needed":true}`
	if string(data) != expectedJSON {
		t.Errorf("JSON output mismatch.\nGot: %s\nWant: %s", string(data), expectedJSON)
	}
}

func TestCommandPayload_JSON(t *testing.T) {
	payload := CommandPayload{
		ID:                 "cmd-123",
		Action:             "patch",
		Packages:           []string{"nginx", "vim"},
		PrePatchScript:     "echo pre",
		PostPatchScript:    "echo post",
		HealthCheckCommand: "check.sh",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal CommandPayload: %v", err)
	}

	var unmarshaled CommandPayload
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal CommandPayload: %v", err)
	}

	if !reflect.DeepEqual(payload, unmarshaled) {
		t.Errorf("CommandPayload mismatch after JSON roundtrip.\nGot: %+v\nWant: %+v", unmarshaled, payload)
	}

	expectedJSON := `{"id":"cmd-123","action":"patch","packages":["nginx","vim"],"pre_patch_script":"echo pre","post_patch_script":"echo post","health_check_command":"check.sh"}`
	if string(data) != expectedJSON {
		t.Errorf("JSON output mismatch.\nGot: %s\nWant: %s", string(data), expectedJSON)
	}
}

func TestErrors(t *testing.T) {
	if ErrAgentOffline.Error() != "agent is offline" {
		t.Errorf("ErrAgentOffline message mismatch. Got: %s, Want: agent is offline", ErrAgentOffline.Error())
	}
}
