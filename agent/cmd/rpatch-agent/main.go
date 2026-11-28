package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/arumes31/rpatch/agent/internal/identity"
	"github.com/arumes31/rpatch/agent/internal/state"
	"github.com/arumes31/rpatch/agent/internal/updater"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gorilla/websocket"
)

var TrustedPubKey = os.Getenv("TRUSTED_PUB_KEY")

// Version is the agent release version, set at build time with
// -ldflags "-X main.Version=<version>". Self-updates refuse manifests that
// are not newer than this version.
var Version = "dev"

const (
	// maxAgentBinaryBytes caps the size of a downloaded agent binary.
	maxAgentBinaryBytes = 256 << 20
	// maxManifestBytes caps the update manifest and its signature.
	maxManifestBytes = 64 << 10
)

// Keep service-owned storage and destructive host operations behind injectable
// boundaries so command tests never depend on or modify the host installation.
var (
	getIdentityFunc  = identity.GetOrGenerate
	loadStateFunc    = state.LoadState
	loadTokensFunc   = loadTokens
	saveStateFunc    = state.SaveState
	clearStateFunc   = state.ClearState
	selfDestructFunc = updater.SelfDestruct
)

// Message types matching server
const (
	MsgTypeHeartbeat = "heartbeat"
	MsgTypeCommand   = "command"
	MsgTypeLog       = "log"
	MsgTypeResult    = "result"
	MsgTypeAck       = "ack"
)

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type HeartbeatPayload struct {
	MacAddress   string `json:"mac_address"`
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	OSVersion    string `json:"os_version"`
	Kernel       string `json:"kernel"`
	BootID       string `json:"boot_id"`
	AgentVersion string `json:"agent_version"`
	RebootNeeded bool   `json:"reboot_needed"`
}

type CommandPayload struct {
	ID                 string   `json:"id"`
	Action             string   `json:"action"`
	Packages           []string `json:"packages,omitempty"`
	PrePatchScript     string   `json:"pre_patch_script,omitempty"`
	PostPatchScript    string   `json:"post_patch_script,omitempty"`
	HealthCheckCommand string   `json:"health_check_command,omitempty"`
	NodeID             string   `json:"node_id,omitempty"`
	ExpiresAt          int64    `json:"expires_at,omitempty"`
	Signature          string   `json:"signature,omitempty"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// getDataDir returns the platform-specific data directory for rpatch.
func getDataDir() string {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("PROGRAMDATA")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "rpatch")
	}
	return "/etc/rpatch"
}

func performSelfDiagnosis() {
	fmt.Println("--- rpatch Agent Self-Diagnosis ---")

	fmt.Print("1. Identity check: ")
	nodeID, err := getIdentityFunc()
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Printf("UUID=%s [OK]\n", nodeID)
	}

	fmt.Print("2. Package Manager check: ")
	pm, err := updater.DetectPackageManager()
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Printf("Detected=%T [OK]\n", pm)
	}

	fmt.Print("3. Disk Space check: ")
	if err := updater.CheckDiskSpace(getDataDir(), 100*1024*1024); err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Println("Available > 100MB [OK]")
	}

	fmt.Print("4. Network (Server) check: ")
	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "localhost:8080"
	}
	req, err := controllerRequest(context.Background(), "GET", serverURL, "/health", nil)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return
	}
	// #nosec G704 -- Request URL comes from validated operator SERVER_URL; TLS is verified and redirects are disabled.
	resp, err := controllerClient(30 * time.Second).Do(req)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Printf("Server Health=%d [OK]\n", resp.StatusCode)
		_ = resp.Body.Close()
	}

	fmt.Println("Self-diagnosis complete.")
}

func getTokenFile() string {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("PROGRAMDATA")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "rpatch", "tokens.json")
	}
	return "/etc/rpatch/tokens.json"
}

func saveTokens(pair TokenPair) error {
	// #nosec G117 -- Intentional credential persistence, written only to an owner-restricted file by writeTokens.
	data, err := json.Marshal(pair)
	if err != nil {
		return err
	}
	return writeTokens(getTokenFile(), data)
}

func loadTokens() (*TokenPair, error) {
	data, err := os.ReadFile(getTokenFile())
	if err != nil {
		return nil, err
	}
	var pair TokenPair
	if err := json.Unmarshal(data, &pair); err != nil {
		return nil, err
	}
	return &pair, nil
}

func refreshTokens(ctx context.Context, serverURL, nodeID string, refreshToken string) (*TokenPair, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"mac":           nodeID,
		"refresh_token": refreshToken,
	})

	req, err := controllerRequest(ctx, "POST", serverURL, "/api/v1/auth/refresh", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := controllerClient(30 * time.Second)
	// #nosec G704 -- Request URL comes from validated operator SERVER_URL; TLS is verified and redirects are disabled.
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed: %d", resp.StatusCode)
	}

	var pair TokenPair
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		return nil, err
	}
	return &pair, nil
}

func main() {
	verifyFlag := flag.Bool("verify", false, "Perform self-diagnosis and exit")
	enrollFlag := flag.Bool("enroll", false, "Redeem the installer's registration grant (RPATCH_* env) and exit")
	flag.Parse()

	if *enrollFlag {
		if err := enroll(context.Background()); err != nil {
			log.Fatalf("Enrollment failed: %v", err)
		}
		return
	}

	if *verifyFlag {
		performSelfDiagnosis()
		return
	}

	_, _ = daemon.SdNotify(false, daemon.SdNotifyReady)
	RunAgent(context.Background())
}

func RunAgent(ctx context.Context) {
	log.Println("Starting rpatch Agent...")

	pm, err := updater.DetectPackageManager()
	if err != nil {
		log.Fatalf("Failed to detect package manager: %v", err)
	}

	nodeID, err := getIdentityFunc()
	if err != nil {
		log.Fatalf("Failed to get agent identity: %v", err)
	}
	log.Printf("Agent Identity (UUID): %s", nodeID)

	if CommandPubKey == "" {
		log.Println("WARNING: COMMAND_PUB_KEY is not set; server commands are executed without signature verification.")
	}

	reportInterruptedJob(func() *state.State { st, _ := loadStateFunc(); return st }())

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "localhost:8080"
	}
	// Refuse to start rather than send credentials to, or take commands from,
	// a controller whose identity cannot be established.
	if _, err := controllerEndpoint(serverURL, ""); err != nil {
		log.Fatalf("Invalid controller configuration: %v", err)
	}

	for {
		err := connectOnce(ctx, serverURL, nodeID, pm)
		if errors.Is(err, errRetryNow) {
			continue
		}
		if err != nil {
			log.Printf("Connection error: %v. Retrying in 30s...", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

var (
	errRetryNow  = errors.New("retry now")
	errReconnect = errors.New("scheduled reconnect")
)

// sessionLifetime forces a periodic reconnect. The reconnect refreshes the
// expired access token, which also rotates the refresh token and extends the
// enrollment claim; without it a connection that stays up longer than the
// refresh token lifetime (7 days) would lose its enrollment.
var sessionLifetime = 12 * time.Hour

func connectOnce(ctx context.Context, serverURL, nodeID string, pm updater.PackageManager) error {
	tokens, err := loadTokensFunc()
	if err != nil || tokens == nil {
		return fmt.Errorf("agent is not enrolled (no tokens in %s); run the installer from the rpatch setup page", getTokenFile())
	}

	u, err := controllerEndpoint(serverURL, "/ws")
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokens.AccessToken)

	// #nosec G706 -- %q escapes newlines and other control characters.
	log.Printf("Connecting to %q", u.String())
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			log.Println("Unauthorized. Attempting token refresh...")
			newTokens, rerr := refreshTokens(ctx, serverURL, nodeID, tokens.RefreshToken)
			if rerr != nil {
				return fmt.Errorf("token refresh failed: %w", rerr)
			}
			if serr := saveTokens(*newTokens); serr != nil {
				return fmt.Errorf("cannot store refreshed tokens: %w", serr)
			}
			return errRetryNow
		}
		return fmt.Errorf("websocket dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	log.Println("Connected to controller")
	return runSession(ctx, conn, nodeID, pm)
}

// runSession reads commands in a goroutine and does all writes itself:
// gorilla/websocket allows only one concurrent writer.
func runSession(ctx context.Context, conn *websocket.Conn, nodeID string, pm updater.PackageManager) error {
	readErr := make(chan error, 1)
	go func() {
		for {
			var wsMsg WSMessage
			if err := conn.ReadJSON(&wsMsg); err != nil {
				readErr <- err
				return
			}
			if wsMsg.Type == MsgTypeAck {
				var ack struct {
					JobID string `json:"job_id"`
				}
				if json.Unmarshal(wsMsg.Payload, &ack) == nil {
					ackResult(ack.JobID)
				}
				continue
			}
			if wsMsg.Type != MsgTypeCommand {
				continue
			}
			var cmd CommandPayload
			if err := json.Unmarshal(wsMsg.Payload, &cmd); err != nil {
				log.Printf("Command unmarshal error: %v", err)
				continue
			}
			// Command fields are untrusted until verified; %q escapes
			// control characters so they cannot forge log lines.
			log.Printf("Received command: %q (Job: %q)", cmd.Action, cmd.ID)
			if err := verifyCommand(cmd, nodeID, time.Now()); err != nil {
				log.Printf("Rejected command %q: %v", cmd.ID, err)
				continue
			}
			go executeCommand(ctx, pm, cmd)
		}
	}()

	write := func(msgType string, payload []byte) error {
		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		return conn.WriteJSON(WSMessage{Type: msgType, Payload: payload})
	}
	sendHeartbeat := func() error {
		data, _ := json.Marshal(newHeartbeat(nodeID, pm))
		return write(MsgTypeHeartbeat, data)
	}
	// Results stay queued until the controller acknowledges them; a result
	// sent again after a reconnect is recorded only once.
	flushResults := func() error {
		for _, data := range pendingResults() {
			if err := write(MsgTypeResult, data); err != nil {
				return err
			}
		}
		return nil
	}

	// Heartbeat first: the controller registers the node on its heartbeat.
	if err := sendHeartbeat(); err != nil {
		return err
	}
	if err := flushResults(); err != nil {
		return err
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	reconnect := time.NewTimer(sessionLifetime)
	defer reconnect.Stop()
	for {
		var err error
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err = <-readErr:
			return err
		case <-reconnect.C:
			return errReconnect
		case <-ticker.C:
			err = sendHeartbeat()
		case <-resultsReady:
			err = flushResults()
		}
		if err != nil {
			return err
		}
	}
}

func newHeartbeat(nodeID string, pm updater.PackageManager) HeartbeatPayload {
	hostname, _ := os.Hostname()
	osVersion, kernel, bootID := hostInfo()
	return HeartbeatPayload{
		MacAddress:   nodeID,
		Hostname:     hostname,
		OS:           runtime.GOOS,
		OSVersion:    osVersion,
		Kernel:       kernel,
		BootID:       bootID,
		AgentVersion: Version,
		RebootNeeded: pm.RebootRequired(),
	}
}

// enroll redeems the one-time registration grant from the installer
// (RPATCH_GROUP, RPATCH_TIMESTAMP, RPATCH_NONCE, RPATCH_SIGNATURE) for a token pair.
func enroll(ctx context.Context) error {
	serverURL := os.Getenv("SERVER_URL")
	if _, err := controllerEndpoint(serverURL, ""); err != nil {
		return fmt.Errorf("invalid SERVER_URL: %w", err)
	}
	nodeID, err := getIdentityFunc()
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	// A reinstall keeps working credentials. Credentials the controller no
	// longer accepts (expired or revoked) are replaced with the fresh grant.
	if t, err := loadTokensFunc(); err == nil && t != nil {
		fresh, rerr := refreshTokens(ctx, serverURL, nodeID, t.RefreshToken)
		if rerr == nil {
			if err := saveTokens(*fresh); err != nil {
				return err
			}
			log.Println("Already enrolled; credentials refreshed.")
			return nil
		}
		log.Printf("Stored credentials were rejected (%v); enrolling with the new grant.", rerr)
	}
	body, _ := json.Marshal(map[string]string{
		"mac":       nodeID,
		"group":     os.Getenv("RPATCH_GROUP"),
		"timestamp": os.Getenv("RPATCH_TIMESTAMP"),
		"nonce":     os.Getenv("RPATCH_NONCE"),
		"signature": os.Getenv("RPATCH_SIGNATURE"),
	})
	req, err := controllerRequest(ctx, "POST", serverURL, "/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// #nosec G704 -- Request URL comes from validated operator SERVER_URL; TLS is verified and redirects are disabled.
	resp, err := controllerClient(30 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("enrollment rejected: %d %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var pair TokenPair
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestBytes)).Decode(&pair); err != nil || pair.AccessToken == "" {
		return fmt.Errorf("invalid enrollment response: %v", err)
	}
	if err := saveTokens(pair); err != nil {
		return err
	}
	log.Printf("Enrolled as node %s", nodeID)
	return nil
}

// agentManifest describes a signed agent release. The Ed25519 signature
// (TRUSTED_PUB_KEY) covers the raw manifest bytes, binding the binary hash to
// a version and platform so an older signed build cannot be replayed.
type agentManifest struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

var performSecureAgentUpdateFunc = func(ctx context.Context) updater.UpdateResult {
	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "localhost:8080"
	}
	client := controllerClient(5 * time.Minute)

	manifestBytes, err := fetchLimited(ctx, client, serverURL, "/download/agent.manifest", maxManifestBytes)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("failed to download manifest: %v", err)}
	}
	sigBytes, err := fetchLimited(ctx, client, serverURL, "/download/agent.manifest.sig", maxManifestBytes)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("failed to download manifest signature: %v", err)}
	}
	manifest, err := verifyManifest(manifestBytes, sigBytes, Version)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("manifest verification failed: %v", err)}
	}

	req, err := controllerRequest(ctx, "GET", serverURL, "/download/agent", nil)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	// #nosec G704 -- Request URL comes from validated operator SERVER_URL; TLS is verified and redirects are disabled.
	resp, err := client.Do(req)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("HTTP %d during agent download", resp.StatusCode)}
	}

	tmpFile, err := os.CreateTemp("", "rpatch-agent-*")
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	// Never read more than the signed size, and verify the signed hash.
	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmpFile, hasher), io.LimitReader(resp.Body, manifest.Size+1))
	if err != nil {
		_ = tmpFile.Close()
		return updater.UpdateResult{Success: false, Error: err}
	}
	if err := tmpFile.Close(); err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	if n != manifest.Size {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("binary size %d does not match manifest size %d", n, manifest.Size)}
	}
	if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), manifest.SHA256) {
		return updater.UpdateResult{Success: false, Error: errors.New("binary hash does not match signed manifest")}
	}

	// #nosec G302 -- This is the Ed25519-verified executable; others may execute but cannot modify it.
	if err := os.Chmod(tmpName, 0755); err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}

	agentPath, err := os.Executable()
	if err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("failed to determine executable path: %v", err)}
	}

	backupPath := agentPath + ".bak." + fmt.Sprint(time.Now().Unix())
	if err := os.Rename(agentPath, backupPath); err != nil && !os.IsNotExist(err) {
		return updater.UpdateResult{Success: false, Error: err}
	}

	if err := os.Rename(tmpName, agentPath); err != nil {
		_ = os.Rename(backupPath, agentPath) // Rollback
		return updater.UpdateResult{Success: false, Error: err}
	}

	log.Printf("Agent updated from %s to %s", Version, manifest.Version)
	out, err := restartAgent(ctx)
	return updater.UpdateResult{Success: err == nil, Output: string(out), Error: err}
}

// fetchLimited downloads a small controller resource, failing if it exceeds limit.
func fetchLimited(ctx context.Context, client *http.Client, serverURL, endpoint string, limit int64) ([]byte, error) {
	req, err := controllerRequest(ctx, "GET", serverURL, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// #nosec G704 -- Request URL comes from validated operator SERVER_URL; TLS is verified and redirects are disabled.
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("response too large")
	}
	return data, nil
}

// verifyManifest checks the manifest signature against TRUSTED_PUB_KEY and
// that it targets this platform with a version newer than currentVersion.
func verifyManifest(manifestBytes, sigBase64 []byte, currentVersion string) (*agentManifest, error) {
	if TrustedPubKey == "" {
		return nil, fmt.Errorf("TRUSTED_PUB_KEY not set: refusing to verify signature")
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(TrustedPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode trusted public key: %v", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size")
	}

	sigBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigBase64)))
	if err != nil {
		return nil, fmt.Errorf("failed to decode signature: %v", err)
	}
	if !ed25519.Verify(pubKeyBytes, manifestBytes, sigBytes) {
		return nil, fmt.Errorf("cryptographic signature verification failed")
	}

	var m agentManifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest: %v", err)
	}
	if m.OS != runtime.GOOS || m.Arch != runtime.GOARCH {
		return nil, fmt.Errorf("manifest targets %s/%s, not %s/%s", m.OS, m.Arch, runtime.GOOS, runtime.GOARCH)
	}
	if m.Size <= 0 || m.Size > maxAgentBinaryBytes {
		return nil, fmt.Errorf("invalid binary size %d", m.Size)
	}
	if len(m.SHA256) != sha256.Size*2 {
		return nil, fmt.Errorf("invalid binary hash")
	}
	if !isNewerVersion(m.Version, currentVersion) {
		return nil, fmt.Errorf("refusing update to version %q: not newer than %q", m.Version, currentVersion)
	}
	return &m, nil
}

// parseVersion parses "v1.2.3" / "1.2.3" into numeric components.
func parseVersion(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

// isNewerVersion reports whether candidate is strictly newer than current.
// Development builds (unparseable current version) accept any valid version.
func isNewerVersion(candidate, current string) bool {
	c, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	cur, ok := parseVersion(current)
	if !ok {
		return true
	}
	for i := 0; i < len(c) || i < len(cur); i++ {
		var a, b int
		if i < len(c) {
			a = c[i]
		}
		if i < len(cur) {
			b = cur[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

func restartAgent(ctx context.Context) ([]byte, error) {
	if runtime.GOOS == "windows" {
		return restartWindowsAgent()
	}
	if _, err := os.Stat("/run/openrc"); err == nil {
		return exec.CommandContext(ctx, "rc-service", "rpatch-agent", "restart").CombinedOutput()
	}
	return exec.CommandContext(ctx, "systemctl", "restart", "rpatch-agent").CombinedOutput()
}
