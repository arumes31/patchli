package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/arumes31/rpatch/server/internal/db"
	"github.com/arumes31/rpatch/server/internal/fleet"
	"github.com/arumes31/rpatch/server/internal/patching"
)

// Test seams.
var (
	listNodes     = db.ListNodes
	openJob       = db.OpenJob
	getJob        = db.GetJob
	listJobs      = db.ListJobs
	isOnline      = fleet.Registry.IsOnline
	dispatch      = patching.Dispatch
	setNodePolicy = db.SetNodePolicy
	getSettings   = db.GetSettings
	saveSettings  = db.SaveSettings
)

// NodeView is a node with its live state for the UI.
type NodeView struct {
	db.Node
	Online bool   `json:"online"`
	Status string `json:"status"`
	JobID  string `json:"job_id,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("Response write failed: %v", err)
	}
}

func nodeViews() ([]NodeView, error) {
	nodes, err := listNodes()
	if err != nil {
		return nil, err
	}
	out := make([]NodeView, 0, len(nodes))
	for _, n := range nodes {
		v := NodeView{Node: n, Online: isOnline(n.ID)}
		if id, err := openJob(n.ID); err == nil && id != "" {
			v.JobID = id
			// A rebooting node is expected to be offline; other jobs of an
			// offline node only finish when it returns.
			if j, err := getJob(id); err == nil && (v.Online || j.Action == "reboot") {
				v.Status = map[string]string{"apply_updates": "patching", "check_updates": "checking", "reboot": "rebooting"}[j.Action]
			}
		}
		if v.Status == "" {
			switch {
			case !v.Online:
				v.Status = "offline"
			case n.RebootRequired:
				v.Status = "reboot required"
			case n.PendingUpdates > 0:
				v.Status = "updates"
			case n.PendingUpdates == 0:
				v.Status = "up to date"
			default:
				v.Status = "unknown"
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// HandleNodes lists all enrolled nodes with their live status.
func HandleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := nodeViews()
	if err != nil {
		log.Printf("Listing nodes failed: %v", err)
		http.Error(w, "Failed to list nodes", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

// HandleStats summarizes the fleet.
func HandleStats(w http.ResponseWriter, r *http.Request) {
	nodes, err := nodeViews()
	if err != nil {
		log.Printf("Listing nodes failed: %v", err)
		http.Error(w, "Failed to list nodes", http.StatusInternalServerError)
		return
	}
	var st struct {
		Total          int `json:"total"`
		Online         int `json:"online"`
		WithUpdates    int `json:"with_updates"`
		PendingUpdates int `json:"pending_updates"`
		RebootRequired int `json:"reboot_required"`
	}
	st.Total = len(nodes)
	for _, n := range nodes {
		if n.Online {
			st.Online++
		}
		if n.PendingUpdates > 0 {
			st.WithUpdates++
			st.PendingUpdates += n.PendingUpdates
		}
		if n.RebootRequired {
			st.RebootRequired++
		}
	}
	writeJSON(w, http.StatusOK, st)
}

// HandleNodeAction starts check_updates, apply_updates or reboot on a node.
func HandleNodeAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || !patching.Actions[req.Action] {
		http.Error(w, "action must be check_updates, apply_updates or reboot", http.StatusBadRequest)
		return
	}
	id, err := dispatch(r.PathValue("id"), req.Action, "manual")
	switch {
	case errors.Is(err, patching.ErrOffline):
		http.Error(w, "Node is offline", http.StatusConflict)
	case errors.Is(err, patching.ErrBusy):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "node already has a running job", "job_id": id})
	case err != nil:
		log.Printf("Dispatch failed: %v", err)
		http.Error(w, "Failed to start job", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": id})
	}
}

// HandleNodePolicy sets auto_reboot, excluded and slot of a node.
func HandleNodePolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AutoReboot bool `json:"auto_reboot"`
		Excluded   bool `json:"excluded"`
		Slot       int  `json:"slot"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || req.Slot < 0 || req.Slot > 10000 {
		http.Error(w, "Invalid policy", http.StatusBadRequest)
		return
	}
	err := setNodePolicy(r.PathValue("id"), req.AutoReboot, req.Excluded, req.Slot)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.NotFound(w, r)
	case err != nil:
		log.Printf("Saving policy failed: %v", err)
		http.Error(w, "Failed to save policy", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// HandleJobs lists recent jobs (?node=<id>&limit=<n>).
func HandleJobs(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 || limit > 500 {
		limit = 100
	}
	jobs, err := listJobs(r.URL.Query().Get("node"), limit)
	if err != nil {
		log.Printf("Listing jobs failed: %v", err)
		http.Error(w, "Failed to list jobs", http.StatusInternalServerError)
		return
	}
	if jobs == nil {
		jobs = []db.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

// HandleJob returns one job including its output.
func HandleJob(w http.ResponseWriter, r *http.Request) {
	j, err := getJob(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// HandleSettings reads (GET) or replaces (PUT) the patch window settings.
func HandleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s, err := getSettings()
		if err != nil {
			http.Error(w, "Failed to read settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, s)
		return
	}
	var s db.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&s); err != nil {
		http.Error(w, "Invalid settings", http.StatusBadRequest)
		return
	}
	start, err1 := time.Parse("15:04", s.WindowStart)
	end, err2 := time.Parse("15:04", s.WindowEnd)
	_, err3 := time.LoadLocation(s.Timezone)
	if err1 != nil || err2 != nil || err3 != nil || s.Timezone == "" || !end.After(start) {
		http.Error(w, "window_start and window_end must be HH:MM with end after start, timezone an IANA name", http.StatusBadRequest)
		return
	}
	if err := saveSettings(s); err != nil {
		http.Error(w, "Failed to save settings", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleWindowStatus reports whether a window is running and when the next starts.
func HandleWindowStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, patching.Default.Status())
}

// HandleWindowRun starts a patch window now.
func HandleWindowRun(w http.ResponseWriter, r *http.Request) {
	if !patching.Default.RunNow() {
		http.Error(w, "A window is already queued", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
