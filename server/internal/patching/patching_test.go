package patching

import (
	"testing"
	"time"

	"github.com/arumes31/rpatch/server/internal/db"
)

func vienna(t *testing.T, s string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Vienna")
	if err != nil {
		t.Fatal(err)
	}
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDue(t *testing.T) {
	set := db.Settings{WindowStart: "03:00", WindowEnd: "05:30", Timezone: "Europe/Vienna", Enabled: true}
	done := "2026-11-28"
	cases := []struct {
		name string
		set  func(db.Settings) db.Settings
		at   string
		want bool
	}{
		{"before window", nil, "2026-11-28 02:59", false},
		{"window start", nil, "2026-11-28 03:00", true},
		{"inside", nil, "2026-11-28 05:29", true},
		{"window end", nil, "2026-11-28 05:30", false},
		{"already ran today", func(s db.Settings) db.Settings { s.LastRunDate = &done; return s }, "2026-11-28 04:00", false},
		{"disabled", func(s db.Settings) db.Settings { s.Enabled = false; return s }, "2026-11-28 04:00", false},
		{"bad timezone", func(s db.Settings) db.Settings { s.Timezone = "Mars/Base"; return s }, "2026-11-28 04:00", false},
		// 03:30 Vienna is 02:30 UTC: the window must use local time.
		{"utc clock", nil, "2026-11-28 03:30", true},
	}
	for _, c := range cases {
		s := set
		if c.set != nil {
			s = c.set(s)
		}
		at := vienna(t, c.at)
		if c.name == "utc clock" {
			at = at.UTC()
		}
		date, got := due(s, at)
		if got != c.want {
			t.Errorf("%s: due=%v want %v", c.name, got, c.want)
		}
		if got && date != "2026-11-28" {
			t.Errorf("%s: date=%s", c.name, date)
		}
	}
}

func TestNextStart(t *testing.T) {
	set := db.Settings{WindowStart: "03:00", WindowEnd: "05:30", Timezone: "Europe/Vienna", Enabled: true}
	for at, want := range map[string]string{
		"2026-11-28 01:00": "2026-11-28 03:00",
		"2026-11-28 04:00": "2026-11-28 03:00", // running now
		"2026-11-28 12:00": "2026-11-29 03:00",
	} {
		got, err := nextStart(set, vienna(t, at))
		if err != nil || !got.Equal(vienna(t, want)) {
			t.Errorf("at %s: got %v %v, want %s", at, got, err, want)
		}
	}
}

func TestSkipReason(t *testing.T) {
	start := vienna(t, "2026-11-28 03:00")
	before, after := start.Add(-time.Hour), start.Add(time.Minute)
	cases := []struct {
		name   string
		n      db.Node
		online bool
		want   string
	}{
		{"excluded", db.Node{Excluded: true}, true, "excluded"},
		{"offline", db.Node{}, false, "offline"},
		{"never patched", db.Node{}, true, ""},
		{"patched yesterday", db.Node{LastPatchAt: &before}, true, ""},
		{"patched this window", db.Node{LastPatchAt: &after}, true, "already patched"},
		{"patched, reboot pending", db.Node{LastPatchAt: &after, RebootRequired: true, AutoReboot: true}, true, ""},
		{"patched, reboot pending, manual reboot", db.Node{LastPatchAt: &after, RebootRequired: true}, true, "already patched"},
	}
	for _, c := range cases {
		if got := skipReason(c.n, c.online, start); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestDispatchRejectsUnknownAndOffline(t *testing.T) {
	old := isOnline
	t.Cleanup(func() { isOnline = old })
	isOnline = func(string) bool { return false }
	if _, err := Dispatch("n1", "rm -rf", "manual"); err == nil {
		t.Error("unknown action accepted")
	}
	if _, err := Dispatch("n1", "reboot", "manual"); err != ErrOffline {
		t.Errorf("offline node: got %v", err)
	}
}
