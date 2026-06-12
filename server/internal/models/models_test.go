package models

import (
	"encoding/json"
	"testing"
)

func TestHeartbeatPayload_Marshal(t *testing.T) {
	p := HeartbeatPayload{
		Hostname: "test",
		MacAddress: "mac",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var p2 HeartbeatPayload
	err = json.Unmarshal(data, &p2)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Hostname != p.Hostname {
		t.Error("mismatch")
	}
}
