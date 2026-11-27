package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
