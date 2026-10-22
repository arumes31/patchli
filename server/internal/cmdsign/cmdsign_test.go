package cmdsign

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/arumes31/patchli/server/internal/models"
)

func TestSignDisabledLeavesCommandUnchanged(t *testing.T) {
	SetKey(nil)
	cmd := models.CommandPayload{ID: "job", Action: "check_updates"}
	got, err := Sign(cmd, "node")
	if err != nil || got.Signature != "" || got.NodeID != "" {
		t.Fatalf("unexpected signing without key: %+v %v", got, err)
	}
}

func TestSignProducesVerifiableSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	SetKey(priv)
	t.Cleanup(func() { SetKey(nil) })

	signed, err := Sign(models.CommandPayload{ID: "job", Action: "apply_updates", PrePatchScript: "echo hi"}, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if signed.NodeID != "node-1" || signed.ExpiresAt == 0 {
		t.Fatalf("command not bound: %+v", signed)
	}
	sig, err := base64.StdEncoding.DecodeString(signed.Signature)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := Message(signed)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(pub, msg, sig) {
		t.Fatal("signature does not verify")
	}
	signed.PrePatchScript = "echo tampered"
	tampered, err := Message(signed)
	if err != nil {
		t.Fatal(err)
	}
	if ed25519.Verify(pub, tampered, sig) {
		t.Fatal("tampered command verified")
	}
}

func TestInit(t *testing.T) {
	t.Cleanup(func() { SetKey(nil) })
	seed := make([]byte, ed25519.SeedSize)
	for _, tc := range []struct {
		name, value string
		wantErr     bool
		enabled     bool
	}{
		{"unset", "", false, false},
		{"seed", base64.StdEncoding.EncodeToString(seed), false, true},
		{"bad base64", "%%%", true, false},
		{"bad size", base64.StdEncoding.EncodeToString([]byte("short")), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			SetKey(nil)
			t.Setenv("COMMAND_SIGNING_KEY", tc.value)
			err := Init()
			if (err != nil) != tc.wantErr || Enabled() != tc.enabled {
				t.Fatalf("err=%v enabled=%v", err, Enabled())
			}
		})
	}
}

// TestMessageWireFormat pins the canonical encoding shared with the agent
// (agent/cmd/patchli-agent/command_auth.go); both sides must produce identical bytes.
func TestMessageWireFormat(t *testing.T) {
	msg, err := Message(models.CommandPayload{ID: "job", Action: "apply_updates", Packages: []string{"a"}, PrePatchScript: "pre", PostPatchScript: "post", HealthCheckCommand: "hc", NodeID: "n", ExpiresAt: 42})
	if err != nil {
		t.Fatal(err)
	}
	want := "patchli-command-v1\n" + `{"id":"job","action":"apply_updates","packages":["a"],"pre_patch_script":"pre","post_patch_script":"post","health_check_command":"hc","node_id":"n","expires_at":42}`
	if string(msg) != want {
		t.Fatalf("got %q\nwant %q", msg, want)
	}
}
