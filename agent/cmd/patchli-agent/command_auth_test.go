package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func signTestCommand(t *testing.T, priv ed25519.PrivateKey, cmd CommandPayload) CommandPayload {
	t.Helper()
	msg, err := commandMessage(cmd)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))
	return cmd
}

func TestVerifyCommand(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := CommandPubKey
	t.Cleanup(func() { CommandPubKey = old })

	now := time.Now()
	base := CommandPayload{ID: "job", Action: "apply_updates", PrePatchScript: "echo hi", NodeID: "node-1", ExpiresAt: now.Add(time.Minute).Unix()}

	CommandPubKey = ""
	if err := verifyCommand(CommandPayload{Action: "check_updates"}, "node-1", now); err != nil {
		t.Fatalf("unsigned command rejected without a configured key: %v", err)
	}

	CommandPubKey = base64.StdEncoding.EncodeToString(pub)
	valid := signTestCommand(t, priv, base)
	if err := verifyCommand(valid, "node-1", now); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}
	if err := verifyCommand(valid, "node-1", now); err == nil {
		t.Fatal("replayed command accepted")
	}

	// A 64-byte signature encodes to 86 Base64 characters plus "==": the last
	// character carries 4 unused bits. Setting them yields different text
	// that lenient decoding maps to the same signature bytes.
	fresh := signTestCommand(t, priv, CommandPayload{ID: "job-2", Action: "check_updates", NodeID: "node-1", ExpiresAt: now.Add(time.Minute).Unix()})
	if err := verifyCommand(fresh, "node-1", now); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}
	alt := fresh
	last := strings.IndexByte(alt.Signature, '=') - 1
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	alt.Signature = alt.Signature[:last] + string(alphabet[strings.IndexByte(alphabet, alt.Signature[last])|1]) + alt.Signature[last+1:]
	if alt.Signature == fresh.Signature {
		t.Fatal("test did not produce an alternative encoding")
	}
	if decoded, err := base64.StdEncoding.DecodeString(alt.Signature); err != nil || string(decoded) != string(mustDecode(t, fresh.Signature)) {
		t.Fatalf("alternative encoding does not decode to the same signature: %v", err)
	}
	if err := verifyCommand(alt, "node-1", now); err == nil {
		t.Fatal("replay with an alternative signature encoding accepted")
	}

	tampered := signTestCommand(t, priv, base)
	tampered.PrePatchScript = "echo tampered"
	expiredCmd := base
	expiredCmd.ExpiresAt = now.Add(-time.Second).Unix()
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	for name, tc := range map[string]struct {
		cmd    CommandPayload
		nodeID string
	}{
		"unsigned":    {base, "node-1"},
		"tampered":    {tampered, "node-1"},
		"other node":  {signTestCommand(t, priv, base), "node-2"},
		"expired":     {signTestCommand(t, priv, expiredCmd), "node-1"},
		"foreign key": {signTestCommand(t, otherPriv, base), "node-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := verifyCommand(tc.cmd, tc.nodeID, now); err == nil {
				t.Fatal("command accepted")
			}
		})
	}
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVerifyManifest(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := TrustedPubKey
	t.Cleanup(func() { TrustedPubKey = old })
	TrustedPubKey = base64.StdEncoding.EncodeToString(pub)

	sum := sha256.Sum256([]byte("binary"))
	manifest := func(version, goos string, size int64) ([]byte, []byte) {
		data, err := json.Marshal(agentManifest{Version: version, OS: goos, Arch: runtime.GOARCH, SHA256: hex.EncodeToString(sum[:]), Size: size})
		if err != nil {
			t.Fatal(err)
		}
		return data, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)))
	}

	data, sig := manifest("1.3.0", runtime.GOOS, 6)
	if _, err := verifyManifest(data, sig, "1.2.9"); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if _, err := verifyManifest(data, sig, "dev"); err != nil {
		t.Fatalf("development build refused update: %v", err)
	}
	type manifestCase struct {
		data, sig []byte
		current   string
	}
	signed := func(version, goos string, size int64) manifestCase {
		d, s := manifest(version, goos, size)
		return manifestCase{d, s, "1.0.0"}
	}
	for name, tc := range map[string]manifestCase{
		"rollback":    {data, sig, "1.3.0"},
		"older":       {data, sig, "v2.0.0"},
		"bad sig":     {data, []byte(base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))), "1.0.0"},
		"tampered":    {append([]byte(" "), data...), sig, "1.0.0"},
		"wrong os":    signed("9.0.0", "plan9", 6),
		"oversized":   signed("9.0.0", runtime.GOOS, maxAgentBinaryBytes+1),
		"bad version": signed("latest", runtime.GOOS, 6),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyManifest(tc.data, tc.sig, tc.current); err == nil {
				t.Fatal("manifest accepted")
			}
		})
	}
}

func TestIsNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		candidate, current string
		want               bool
	}{
		{"1.2.4", "1.2.3", true},
		{"v1.10.0", "1.9.9", true},
		{"1.2.3", "1.2.3", false},
		{"1.2", "1.2.0", false},
		{"1.2.3-rc1", "1.2.2", true},
		{"1.0.0", "dev", true},
		{"garbage", "1.0.0", false},
	} {
		if got := isNewerVersion(tc.candidate, tc.current); got != tc.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}

// TestCommandMessageWireFormat pins the canonical encoding shared with the
// server (server/internal/cmdsign); both sides must produce identical bytes.
func TestCommandMessageWireFormat(t *testing.T) {
	msg, err := commandMessage(CommandPayload{ID: "job", Action: "apply_updates", Packages: []string{"a"}, PrePatchScript: "pre", PostPatchScript: "post", HealthCheckCommand: "hc", NodeID: "n", ExpiresAt: 42})
	if err != nil {
		t.Fatal(err)
	}
	want := "patchli-command-v1\n" + `{"id":"job","action":"apply_updates","packages":["a"],"pre_patch_script":"pre","post_patch_script":"post","health_check_command":"hc","node_id":"n","expires_at":42}`
	if string(msg) != want {
		t.Fatalf("got %q\nwant %q", msg, want)
	}
}
