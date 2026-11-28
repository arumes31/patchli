package db

import (
	"database/sql"
	"errors"
	"time"
)

var errNoDB = errors.New("database not initialized")

// Node is a managed host as shown in the UI and used by the scheduler.
type Node struct {
	ID             string     `json:"id"`
	Hostname       string     `json:"hostname"`
	OS             string     `json:"os"`
	OSVersion      string     `json:"os_version"`
	Kernel         string     `json:"kernel"`
	AgentVersion   string     `json:"agent_version"`
	LastHeartbeat  time.Time  `json:"last_heartbeat"`
	RebootRequired bool       `json:"reboot_required"`
	PendingUpdates int        `json:"pending_updates"`
	LastCheckAt    *time.Time `json:"last_check_at"`
	LastPatchAt    *time.Time `json:"last_patch_at"`
	AutoReboot     bool       `json:"auto_reboot"`
	Excluded       bool       `json:"excluded"`
	Slot           int        `json:"slot"`
	BootID         string     `json:"-"`
}

// Job is one command sent to an agent.
type Job struct {
	ID           string     `json:"id"`
	NodeID       string     `json:"node_id"`
	Hostname     string     `json:"hostname"`
	Action       string     `json:"action"`
	State        string     `json:"state"`
	Trigger      string     `json:"trigger"`
	Error        string     `json:"error,omitempty"`
	Output       string     `json:"output,omitempty"`
	RebootNeeded *bool      `json:"reboot_needed"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

// Settings holds the daily patch window.
type Settings struct {
	WindowStart string  `json:"window_start"` // HH:MM
	WindowEnd   string  `json:"window_end"`   // HH:MM
	Timezone    string  `json:"timezone"`
	Enabled     bool    `json:"enabled"`
	LastRunDate *string `json:"last_run_date"`
}

const nodeColumns = `mac_address, hostname, COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''),
	COALESCE(agent_version,''), COALESCE(last_heartbeat, to_timestamp(0)), COALESCE(reboot_required,false), pending_updates,
	last_check_at, last_patch_at, auto_reboot, excluded, slot, COALESCE(boot_id,'')`

func scanNode(row interface{ Scan(...any) error }) (Node, error) {
	var n Node
	err := row.Scan(&n.ID, &n.Hostname, &n.OS, &n.OSVersion, &n.Kernel, &n.AgentVersion, &n.LastHeartbeat,
		&n.RebootRequired, &n.PendingUpdates, &n.LastCheckAt, &n.LastPatchAt, &n.AutoReboot, &n.Excluded, &n.Slot, &n.BootID)
	return n, err
}

// ListNodes returns enrolled nodes in patch order (slot, then hostname).
func ListNodes() ([]Node, error) {
	if DB == nil {
		return nil, errNoDB
	}
	rows, err := DB.Query(`SELECT ` + nodeColumns + ` FROM nodes
		WHERE mac_address IN (SELECT mac_address FROM node_enrollments) ORDER BY slot, hostname`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetNode returns one node.
func GetNode(id string) (Node, error) {
	if DB == nil {
		return Node{}, errNoDB
	}
	return scanNode(DB.QueryRow(`SELECT `+nodeColumns+` FROM nodes WHERE mac_address = $1`, id))
}

// UpdateHeartbeat stores the agent's self-reported state.
func UpdateHeartbeat(id, hostname, osName, osVersion, kernel, bootID, agentVersion string, rebootRequired bool) error {
	if DB == nil {
		return errNoDB
	}
	_, err := DB.Exec(`INSERT INTO nodes (mac_address, hostname, os_name, os_version, kernel_version, boot_id, agent_version, reboot_required, status, last_heartbeat)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'online', CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE SET hostname = EXCLUDED.hostname, os_name = EXCLUDED.os_name,
			os_version = EXCLUDED.os_version, kernel_version = EXCLUDED.kernel_version, boot_id = EXCLUDED.boot_id,
			agent_version = EXCLUDED.agent_version, reboot_required = EXCLUDED.reboot_required,
			status = 'online', last_heartbeat = CURRENT_TIMESTAMP`,
		id, hostname, osName, osVersion, kernel, bootID, agentVersion, rebootRequired)
	return err
}

// SetNodePolicy updates the per-node patch policy.
func SetNodePolicy(id string, autoReboot, excluded bool, slot int) error {
	if DB == nil {
		return errNoDB
	}
	res, err := DB.Exec(`UPDATE nodes SET auto_reboot = $2, excluded = $3, slot = $4 WHERE mac_address = $1`, id, autoReboot, excluded, slot)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateJob records a job before it is sent. bootID is the node's boot ID at
// that moment, used to confirm a reboot.
func CreateJob(nodeID, action, trigger, bootID string) (string, error) {
	if DB == nil {
		return "", errNoDB
	}
	var id string
	err := DB.QueryRow(`INSERT INTO jobs (node_id, action, trigger, boot_id_before) VALUES ($1, $2, $3, NULLIF($4::varchar,'')) RETURNING id`,
		nodeID, action, trigger, bootID).Scan(&id)
	return id, err
}

// FailJob marks an open job as failed.
func FailJob(id, reason string) error {
	if DB == nil {
		return errNoDB
	}
	_, err := DB.Exec(`UPDATE jobs SET state = 'failed', error = $2, finished_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND state IN ('sent', 'rebooting')`, id, reason)
	return err
}

// RecordResult stores an agent's result for one of its own jobs and updates
// the node. A successful reboot result only moves the job to "rebooting";
// the job completes when the node returns with a new boot ID.
func RecordResult(nodeID, jobID, action string, success bool, errText, output string, rebootNeeded bool, pending int) error {
	if DB == nil {
		return errNoDB
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	state := "failed"
	switch {
	case success && action == "reboot":
		state = "rebooting"
	case success:
		state = "ok"
	}
	res, err := tx.Exec(`UPDATE jobs SET state = $3::varchar, error = NULLIF($4::text,''), output = $5::text, reboot_needed = $6::bool,
			finished_at = CASE WHEN $3::varchar = 'rebooting' THEN NULL ELSE CURRENT_TIMESTAMP END
		WHERE id = $1 AND node_id = $2 AND state <> 'ok'`, jobID, nodeID, state, errText, output, rebootNeeded)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`UPDATE nodes SET reboot_required = $2::bool,
			pending_updates = CASE WHEN $3::int >= 0 THEN $3::int ELSE pending_updates END,
			last_check_at = CASE WHEN $4::bool AND $5::text IN ('check_updates','apply_updates') THEN CURRENT_TIMESTAMP ELSE last_check_at END,
			last_patch_at = CASE WHEN $4::bool AND $5::text = 'apply_updates' THEN CURRENT_TIMESTAMP ELSE last_patch_at END
		WHERE mac_address = $1`, nodeID, rebootNeeded, pending, success, action); err != nil {
		return err
	}
	return tx.Commit()
}

// ConfirmReboots completes open reboot jobs of a node that came back with a
// different boot ID.
func ConfirmReboots(nodeID, bootID string) error {
	if DB == nil || bootID == "" {
		return nil
	}
	_, err := DB.Exec(`UPDATE jobs SET state = 'ok', error = NULL, finished_at = CURRENT_TIMESTAMP
		WHERE node_id = $1 AND action = 'reboot' AND state IN ('sent', 'rebooting')
		AND boot_id_before IS NOT NULL AND boot_id_before <> $2`, nodeID, bootID)
	return err
}

// GetJob returns one job.
func GetJob(id string) (Job, error) {
	if DB == nil {
		return Job{}, errNoDB
	}
	var j Job
	var errText, output sql.NullString
	err := DB.QueryRow(`SELECT j.id, j.node_id, COALESCE(n.hostname,''), j.action, j.state, j.trigger, j.error, j.output,
			j.reboot_needed, j.created_at, j.finished_at
		FROM jobs j LEFT JOIN nodes n ON n.mac_address = j.node_id WHERE j.id = $1`, id).
		Scan(&j.ID, &j.NodeID, &j.Hostname, &j.Action, &j.State, &j.Trigger, &errText, &output, &j.RebootNeeded, &j.CreatedAt, &j.FinishedAt)
	j.Error, j.Output = errText.String, output.String
	return j, err
}

// ListJobs returns recent jobs, newest first, optionally for one node. The
// output is left out; GetJob returns it.
func ListJobs(nodeID string, limit int) ([]Job, error) {
	if DB == nil {
		return nil, errNoDB
	}
	rows, err := DB.Query(`SELECT j.id, j.node_id, COALESCE(n.hostname,''), j.action, j.state, j.trigger, COALESCE(j.error,''),
			j.reboot_needed, j.created_at, j.finished_at
		FROM jobs j LEFT JOIN nodes n ON n.mac_address = j.node_id
		WHERE ($1::varchar = '' OR j.node_id = $1::varchar) ORDER BY j.created_at DESC LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.NodeID, &j.Hostname, &j.Action, &j.State, &j.Trigger, &j.Error, &j.RebootNeeded, &j.CreatedAt, &j.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// OpenJob returns the ID of a node job that has not finished, or "".
func OpenJob(nodeID string) (string, error) {
	if DB == nil {
		return "", errNoDB
	}
	var id string
	err := DB.QueryRow(`SELECT id FROM jobs WHERE node_id = $1 AND state IN ('sent','rebooting')
		ORDER BY created_at DESC LIMIT 1`, nodeID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// ExpireJobs fails jobs whose result never arrived: reboots after 20
// minutes, everything else after 2 hours.
func ExpireJobs() error {
	if DB == nil {
		return errNoDB
	}
	_, err := DB.Exec(`UPDATE jobs SET state = 'failed', error = 'timeout: no result from the agent', finished_at = CURRENT_TIMESTAMP
		WHERE (state IN ('sent','rebooting') AND action = 'reboot' AND created_at < CURRENT_TIMESTAMP - INTERVAL '20 minutes')
		   OR (state = 'sent' AND created_at < CURRENT_TIMESTAMP - INTERVAL '2 hours')`)
	return err
}

// GetSettings returns the patch window settings.
func GetSettings() (Settings, error) {
	if DB == nil {
		return Settings{}, errNoDB
	}
	var s Settings
	err := DB.QueryRow(`SELECT to_char(window_start,'HH24:MI'), to_char(window_end,'HH24:MI'), timezone, enabled,
		to_char(last_run_date,'YYYY-MM-DD') FROM settings WHERE id = 1`).Scan(&s.WindowStart, &s.WindowEnd, &s.Timezone, &s.Enabled, &s.LastRunDate)
	return s, err
}

// SaveSettings stores the patch window settings (not last_run_date).
func SaveSettings(s Settings) error {
	if DB == nil {
		return errNoDB
	}
	_, err := DB.Exec(`UPDATE settings SET window_start = $1::time, window_end = $2::time, timezone = $3, enabled = $4 WHERE id = 1`,
		s.WindowStart, s.WindowEnd, s.Timezone, s.Enabled)
	return err
}

// MarkWindowDone records that the window of the given local date completed.
func MarkWindowDone(date string) error {
	if DB == nil {
		return errNoDB
	}
	_, err := DB.Exec(`UPDATE settings SET last_run_date = $1::date WHERE id = 1`, date)
	return err
}
