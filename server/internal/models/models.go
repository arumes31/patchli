package models

import (
	"errors"
	"time"
)

var ErrAgentOffline = errors.New("agent is offline")

type AgentDetails struct {
	Hostname      string `json:"hostname"`
	MacAddress    string `json:"mac_address"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	Status        string `json:"status"`
	LastHeartbeat string `json:"last_heartbeat"`
}

type HeartbeatPayload struct {
	MacAddress   string `json:"mac_address"`
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	OSVersion    string `json:"os_version"`
	Kernel       string `json:"kernel"`
	RebootNeeded bool   `json:"reboot_needed"`
}

type CommandPayload struct {
	ID                 string   `json:"id"`
	Action             string   `json:"action"` // e.g., "patch", "reboot", "chaos_restart"
	Packages           []string `json:"packages,omitempty"`
	PrePatchScript     string   `json:"pre_patch_script,omitempty"`
	PostPatchScript    string   `json:"post_patch_script,omitempty"`
	HealthCheckCommand string   `json:"health_check_command,omitempty"`
}

type Job struct {
	ID                 string
	NodeMac            string
	GroupID            int
	Action             string
	Packages           []string
	PrePatchScript     string
	PostPatchScript    string
	HealthCheckCommand string
	MaintenanceWindow  *MaintenanceWindow
	StartTime          time.Time
}

type MaintenanceWindow struct {
	StartTime time.Time
	EndTime   time.Time
}
