// Package cmdsign signs server-to-agent commands with an Ed25519 key so that
// agents configured with COMMAND_PUB_KEY can reject commands that were not
// issued by the control plane, or that were replayed to another node.
package cmdsign

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/arumes31/patchli/server/internal/models"
)

// CommandLifetime bounds how long a signed command stays valid.
const CommandLifetime = 15 * time.Minute

// signaturePrefix domain-separates command signatures from other signatures
// made with the same key. The agent uses the identical prefix.
const signaturePrefix = "patchli-command-v1\n"

var (
	mu         sync.RWMutex
	signingKey ed25519.PrivateKey
)

// Init loads COMMAND_SIGNING_KEY (base64 Ed25519 seed or private key). An
// empty value disables signing; an invalid value is an error.
func Init() error {
	raw := os.Getenv("COMMAND_SIGNING_KEY")
	if raw == "" {
		SetKey(nil)
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return errors.New("COMMAND_SIGNING_KEY is not valid base64")
	}
	switch len(b) {
	case ed25519.SeedSize:
		SetKey(ed25519.NewKeyFromSeed(b))
	case ed25519.PrivateKeySize:
		SetKey(ed25519.PrivateKey(b))
	default:
		return errors.New("COMMAND_SIGNING_KEY must be a 32-byte seed or 64-byte Ed25519 private key")
	}
	return nil
}

// SetKey replaces the signing key (nil disables signing).
func SetKey(key ed25519.PrivateKey) {
	mu.Lock()
	defer mu.Unlock()
	signingKey = key
}

// Enabled reports whether commands are signed.
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return signingKey != nil
}

type signedFields struct {
	ID                 string   `json:"id"`
	Action             string   `json:"action"`
	Packages           []string `json:"packages"`
	PrePatchScript     string   `json:"pre_patch_script"`
	PostPatchScript    string   `json:"post_patch_script"`
	HealthCheckCommand string   `json:"health_check_command"`
	NodeID             string   `json:"node_id"`
	ExpiresAt          int64    `json:"expires_at"`
}

// Message returns the canonical bytes covered by a command signature.
func Message(cmd models.CommandPayload) ([]byte, error) {
	pkgs := cmd.Packages
	if pkgs == nil {
		pkgs = []string{}
	}
	data, err := json.Marshal(signedFields{
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
	return append([]byte(signaturePrefix), data...), nil
}

// Sign binds cmd to nodeID, sets its expiry and signs it. Without a key the
// command is returned unchanged.
func Sign(cmd models.CommandPayload, nodeID string) (models.CommandPayload, error) {
	mu.RLock()
	key := signingKey
	mu.RUnlock()
	if key == nil {
		return cmd, nil
	}
	cmd.NodeID = nodeID
	cmd.ExpiresAt = time.Now().Add(CommandLifetime).Unix()
	cmd.Signature = ""
	msg, err := Message(cmd)
	if err != nil {
		return cmd, err
	}
	cmd.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, msg))
	return cmd, nil
}
