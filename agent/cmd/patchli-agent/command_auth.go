package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// CommandPubKey is the base64 Ed25519 public key matching the server's
// COMMAND_SIGNING_KEY. When set, only commands signed for this node and not
// yet expired are executed.
var CommandPubKey = os.Getenv("COMMAND_PUB_KEY")

// commandSignaturePrefix must match the server (internal/cmdsign).
const commandSignaturePrefix = "patchli-command-v1\n"

// commandSignedFields mirrors the server's canonical command encoding. Field
// order and JSON names must stay identical on both sides.
type commandSignedFields struct {
	ID                 string   `json:"id"`
	Action             string   `json:"action"`
	Packages           []string `json:"packages"`
	PrePatchScript     string   `json:"pre_patch_script"`
	PostPatchScript    string   `json:"post_patch_script"`
	HealthCheckCommand string   `json:"health_check_command"`
	NodeID             string   `json:"node_id"`
	ExpiresAt          int64    `json:"expires_at"`
}

func commandMessage(cmd CommandPayload) ([]byte, error) {
	pkgs := cmd.Packages
	if pkgs == nil {
		pkgs = []string{}
	}
	data, err := json.Marshal(commandSignedFields{
		ID:                 cmd.ID,
		Action:             cmd.Action,
		Packages:           pkgs,
		PrePatchScript:     cmd.PrePatchScript,
		PostPatchScript:    cmd.PostPatchScript,
		HealthCheckCommand: cmd.HealthCheckCommand,
		NodeID:             cmd.NodeID,
		ExpiresAt:          cmd.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	return append([]byte(commandSignaturePrefix), data...), nil
}

var (
	seenCommandsMu sync.Mutex
	seenCommands   = map[string]time.Time{}
)

// verifyCommand enforces command signatures when COMMAND_PUB_KEY is set.
// Without a key, commands are accepted for backwards compatibility (a warning
// is logged at startup); controllerEndpoint only allows that over verified
// TLS.
func verifyCommand(cmd CommandPayload, nodeID string, now time.Time) error {
	if CommandPubKey == "" {
		return nil
	}
	pub, err := base64.StdEncoding.DecodeString(CommandPubKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid COMMAND_PUB_KEY")
	}
	if cmd.Signature == "" {
		return errors.New("command is not signed")
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(cmd.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("malformed command signature")
	}
	msg, err := commandMessage(cmd)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, msg, sig) {
		return errors.New("command signature verification failed")
	}
	if cmd.NodeID != nodeID {
		return fmt.Errorf("command addressed to node %q", cmd.NodeID)
	}
	expires := time.Unix(cmd.ExpiresAt, 0)
	if !now.Before(expires) {
		return errors.New("command expired")
	}

	// Reject replays of the same signed command within its lifetime. The
	// cache is keyed by the verified signature bytes, not their text, so an
	// alternative encoding of a seen signature is still a replay.
	replayKey := string(sig)
	seenCommandsMu.Lock()
	defer seenCommandsMu.Unlock()
	for k, exp := range seenCommands {
		if !now.Before(exp) {
			delete(seenCommands, k)
		}
	}
	if _, seen := seenCommands[replayKey]; seen {
		return errors.New("command replayed")
	}
	seenCommands[replayKey] = expires
	return nil
}
