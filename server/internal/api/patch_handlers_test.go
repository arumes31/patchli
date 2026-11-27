package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/arumes31/rpatch/server/internal/db"
	"github.com/arumes31/rpatch/server/internal/patching"
)

func stubFleet(t *testing.T, nodes []db.Node, online map[string]bool, open map[string]db.Job) {
	t.Helper()
	oldList, oldOnline, oldOpen, oldGet := listNodes, isOnline, openJob, getJob
	t.Cleanup(func() { listNodes, isOnline, openJob, getJob = oldList, oldOnline, oldOpen, oldGet })
	listNodes = func() ([]db.Node, error) { return nodes, nil }
	isOnline = func(id string) bool { return online[id] }
	openJob = func(id string) (string, error) { return open[id].ID, nil }
	getJob = func(id string) (db.Job, error) {
		for _, j := range open {
			if j.ID == id {
				return j, nil
			}
		}
		return db.Job{}, nil
	}
}

func TestHandleNodesStatus(t *testing.T) {
	stubFleet(t, []db.Node{
		{ID: "a", PendingUpdates: 0},
		{ID: "b", PendingUpdates: 4},
		{ID: "c", RebootRequired: true},
		{ID: "d"},
		{ID: "e", PendingUpdates: 2},
		{ID: "f", PendingUpdates: -1},
	}, map[string]bool{"a": true, "b": true, "c": true, "e": true, "f": true},
		map[string]db.Job{"e": {ID: "j1", Action: "apply_updates"}})

	rr := httptest.NewRecorder()
	HandleNodes(rr, httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil))
	var got []NodeView
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []string{"up to date", "updates", "reboot required", "offline", "patching", "unknown"}
	for i, w := range want {
		if got[i].Status != w {
			t.Errorf("node %s: status %q want %q", got[i].ID, got[i].Status, w)
		}
	}

	rr = httptest.NewRecorder()
	HandleStats(rr, httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil))
	var st map[string]int
	_ = json.Unmarshal(rr.Body.Bytes(), &st)
	if st["total"] != 6 || st["online"] != 5 || st["with_updates"] != 2 || st["pending_updates"] != 6 || st["reboot_required"] != 1 {
		t.Errorf("stats %v", st)
	}
}

func TestHandleNodeAction(t *testing.T) {
	old := dispatch
	t.Cleanup(func() { dispatch = old })
	var gotNode, gotAction string
	dispatch = func(node, action, trigger string) (string, error) {
		gotNode, gotAction = node, action
		switch node {
		case "off":
			return "", patching.ErrOffline
		case "busy":
			return "j0", patching.ErrBusy
		}
		return "j1", nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes/{id}/action", HandleNodeAction)
	do := func(node, body string) int {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/nodes/"+node+"/action", strings.NewReader(body)))
		return rr.Code
	}
	if c := do("n1", `{"action":"apply_updates"}`); c != http.StatusAccepted || gotNode != "n1" || gotAction != "apply_updates" {
		t.Errorf("patch: %d %s %s", c, gotNode, gotAction)
	}
	if c := do("n1", `{"action":"self_destruct"}`); c != http.StatusBadRequest {
		t.Errorf("non-operator action must be rejected, got %d", c)
	}
	if c := do("off", `{"action":"reboot"}`); c != http.StatusConflict {
		t.Errorf("offline: %d", c)
	}
	if c := do("busy", `{"action":"reboot"}`); c != http.StatusConflict {
		t.Errorf("busy: %d", c)
	}
}

func TestHandleSettingsValidation(t *testing.T) {
	old := saveSettings
	t.Cleanup(func() { saveSettings = old })
	var saved *db.Settings
	saveSettings = func(s db.Settings) error { saved = &s; return nil }
	for body, want := range map[string]int{
		`{"window_start":"03:00","window_end":"05:30","timezone":"Europe/Vienna","enabled":true}`: http.StatusNoContent,
		`{"window_start":"05:00","window_end":"03:00","timezone":"Europe/Vienna","enabled":true}`: http.StatusBadRequest,
		`{"window_start":"3am","window_end":"05:30","timezone":"Europe/Vienna","enabled":true}`:   http.StatusBadRequest,
		`{"window_start":"03:00","window_end":"05:30","timezone":"Mars/Base","enabled":true}`:     http.StatusBadRequest,
	} {
		saved = nil
		rr := httptest.NewRecorder()
		HandleSettings(rr, httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(body)))
		if rr.Code != want || (want == http.StatusNoContent) != (saved != nil) {
			t.Errorf("%s: got %d (saved=%v)", body, rr.Code, saved != nil)
		}
	}
}

func TestHandleNodePolicy(t *testing.T) {
	old := setNodePolicy
	t.Cleanup(func() { setNodePolicy = old })
	var got [3]any
	setNodePolicy = func(id string, autoReboot, excluded bool, slot int) error {
		if id == "missing" {
			return sql.ErrNoRows
		}
		got = [3]any{autoReboot, excluded, slot}
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/nodes/{id}/policy", HandleNodePolicy)
	do := func(id, body string) int {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/nodes/"+id+"/policy", strings.NewReader(body)))
		return rr.Code
	}
	if c := do("n1", `{"auto_reboot":false,"excluded":true,"slot":999}`); c != http.StatusNoContent || got != [3]any{false, true, 999} {
		t.Errorf("save: %d %v", c, got)
	}
	if c := do("n1", `{"slot":-1}`); c != http.StatusBadRequest {
		t.Errorf("negative slot: %d", c)
	}
	if c := do("missing", `{"slot":1}`); c != http.StatusNotFound {
		t.Errorf("unknown node: %d", c)
	}
}

func TestHandleJobsAndJob(t *testing.T) {
	oldList, oldGet := listJobs, getJob
	t.Cleanup(func() { listJobs, getJob = oldList, oldGet })
	var gotNode string
	var gotLimit int
	listJobs = func(node string, limit int) ([]db.Job, error) { gotNode, gotLimit = node, limit; return nil, nil }
	getJob = func(id string) (db.Job, error) {
		if id == "j1" {
			return db.Job{ID: "j1", Output: "apt output"}, nil
		}
		return db.Job{}, sql.ErrNoRows
	}
	rr := httptest.NewRecorder()
	HandleJobs(rr, httptest.NewRequest(http.MethodGet, "/api/v1/jobs?node=n1&limit=9999", nil))
	if rr.Body.String() != "[]\n" || gotNode != "n1" || gotLimit != 100 {
		t.Errorf("jobs: %q node=%s limit=%d", rr.Body.String(), gotNode, gotLimit)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/{id}", HandleJob)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/j1", nil))
	if !strings.Contains(rr.Body.String(), "apt output") {
		t.Errorf("job: %s", rr.Body.String())
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/nope", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("missing job: %d", rr.Code)
	}
}

func TestHandleSettingsGetAndWindow(t *testing.T) {
	old := getSettings
	t.Cleanup(func() { getSettings = old })
	getSettings = func() (db.Settings, error) {
		return db.Settings{WindowStart: "03:00", WindowEnd: "05:30", Timezone: "Europe/Vienna", Enabled: true}, nil
	}
	rr := httptest.NewRecorder()
	HandleSettings(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if !strings.Contains(rr.Body.String(), `"timezone":"Europe/Vienna"`) {
		t.Errorf("settings: %s", rr.Body.String())
	}
	rr = httptest.NewRecorder()
	HandleWindowStatus(rr, httptest.NewRequest(http.MethodGet, "/api/v1/window", nil))
	if !strings.Contains(rr.Body.String(), `"running":false`) {
		t.Errorf("window: %s", rr.Body.String())
	}
	rr = httptest.NewRecorder()
	HandleWindowRun(rr, httptest.NewRequest(http.MethodPost, "/api/v1/window/run", nil))
	first := rr.Code
	rr = httptest.NewRecorder()
	HandleWindowRun(rr, httptest.NewRequest(http.MethodPost, "/api/v1/window/run", nil))
	if first != http.StatusAccepted || rr.Code != http.StatusConflict {
		t.Errorf("run now: %d then %d", first, rr.Code)
	}
}

func TestHandleAgentDownload(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/rpatch-agent-linux-amd64", []byte("ELF"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_DIR", dir)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /download/{file}", HandleAgentDownload)
	for path, want := range map[string]int{
		"/download/rpatch-agent-linux-amd64": http.StatusOK,
		"/download/rpatch-agent-linux-arm64": http.StatusNotFound, // not built
		"/download/agent.manifest":           http.StatusNotFound,
		"/download/..%2Fsecret":              http.StatusNotFound,
	} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != want {
			t.Errorf("%s: %d want %d", path, rr.Code, want)
		}
	}
}
