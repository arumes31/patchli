package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
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
	"strings"
	"time"

	"github.com/arumes31/patchli/agent/internal/control"
	"github.com/arumes31/patchli/agent/internal/identity"
	"github.com/arumes31/patchli/agent/internal/state"
	"github.com/arumes31/patchli/agent/internal/updater"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gorilla/websocket"
)

// shellCommandContext returns an exec.Cmd that runs a shell script cross-platform.
// On Windows it uses cmd.exe /c, on Unix it uses sh -c.
func shellCommandContext(ctx context.Context, script string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// #nosec G204 -- scripts are an explicit patch-orchestration feature, accepted only over the authenticated WSS channel and gated by ALLOW_REMOTE_SCRIPTS.
		return exec.CommandContext(ctx, "cmd.exe", "/c", script)
	}
	// #nosec G204 -- scripts are an explicit patch-orchestration feature, accepted only over the authenticated WSS channel and gated by ALLOW_REMOTE_SCRIPTS.
	return exec.CommandContext(ctx, "sh", "-c", script)
}

var TrustedPubKey = os.Getenv("TRUSTED_PUB_KEY")

// Message types matching server
const (
	MsgTypeHeartbeat = "heartbeat"
	MsgTypeCommand   = "command"
	MsgTypeLog       = "log"
	MsgTypeResult    = "result"
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
	RebootNeeded bool   `json:"reboot_needed"`
}

type CommandPayload struct {
	ID                 string   `json:"id"`
	Action             string   `json:"action"`
	Packages           []string `json:"packages,omitempty"`
	PrePatchScript     string   `json:"pre_patch_script,omitempty"`
	PostPatchScript    string   `json:"post_patch_script,omitempty"`
	HealthCheckCommand string   `json:"health_check_command,omitempty"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// getDataDir returns the platform-specific data directory for Patchli.
func getDataDir() string {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("PROGRAMDATA")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "Patchli")
	}
	return "/etc/patchli"
}

func performSelfDiagnosis() {
	fmt.Println("--- Patchli Agent Self-Diagnosis ---")

	fmt.Print("1. Identity check: ")
	nodeID, err := identity.GetOrGenerate()
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
	controlClient, err := control.FromEnvironment(os.Getenv)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		fmt.Println("Self-diagnosis complete.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := controlClient.NewRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		fmt.Println("Self-diagnosis complete.")
		return
	}
	resp, err := controlClient.Do(request)
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
		return filepath.Join(pd, "Patchli", "tokens.json")
	}
	return "/etc/patchli/tokens.json"
}

func saveTokens(pair TokenPair) error {
	// #nosec G117 -- credentials are intentionally persisted to a mode-0600 administrator-owned token file.
	data, err := json.Marshal(pair)
	if err != nil {
		return err
	}
	dir := filepath.Dir(getTokenFile())
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}
	return os.WriteFile(getTokenFile(), data, 0600)
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

func refreshTokens(ctx context.Context, controlClient *control.Client, nodeID string, refreshToken string) (*TokenPair, error) {
	reqBody, err := json.Marshal(map[string]string{
		"mac":           nodeID,
		"refresh_token": refreshToken,
	})
	if err != nil {
		return nil, fmt.Errorf("encode refresh request: %w", err)
	}

	req, err := controlClient.NewRequest(ctx, http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := controlClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed: %d", resp.StatusCode)
	}

	var pair TokenPair
	if err := json.NewDecoder(control.LimitedBody(resp.Body)).Decode(&pair); err != nil {
		return nil, err
	}
	return &pair, nil
}

func registerAgent(ctx context.Context, controlClient *control.Client, nodeID string, getenv func(string) string) (*TokenPair, error) {
	payload := map[string]string{
		"mac":       nodeID,
		"group":     strings.TrimSpace(getenv("REGISTRATION_GROUP")),
		"timestamp": strings.TrimSpace(getenv("REGISTRATION_TIMESTAMP")),
		"signature": strings.TrimSpace(getenv("REGISTRATION_SIGNATURE")),
	}
	if payload["group"] == "" || payload["timestamp"] == "" || payload["signature"] == "" {
		return nil, errors.New("agent is not enrolled; REGISTRATION_GROUP, REGISTRATION_TIMESTAMP, and REGISTRATION_SIGNATURE are required once")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode registration request: %w", err)
	}
	request, err := controlClient.NewRequest(ctx, http.MethodPost, "/api/v1/auth/login", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := controlClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registration failed with status %d", response.StatusCode)
	}
	var pair TokenPair
	if err := json.NewDecoder(control.LimitedBody(response.Body)).Decode(&pair); err != nil {
		return nil, fmt.Errorf("decode registration response: %w", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		return nil, errors.New("registration response omitted credentials")
	}
	return &pair, nil
}

func main() {
	verifyFlag := flag.Bool("verify", false, "Perform self-diagnosis and exit")
	flag.Parse()

	if *verifyFlag {
		performSelfDiagnosis()
		return
	}

	_, _ = daemon.SdNotify(false, daemon.SdNotifyReady)
	if err := RunAgent(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func RunAgent(ctx context.Context) error {
	log.Println("Starting Patchli Agent...")
	controlClient, err := control.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}

	pm, err := updater.DetectPackageManager()
	if err != nil {
		return fmt.Errorf("detect package manager: %w", err)
	}

	nodeID, err := identity.GetOrGenerate()
	if err != nil {
		return fmt.Errorf("get agent identity: %w", err)
	}
	log.Printf("Agent Identity (UUID): %s", nodeID)

	// State Recovery on Boot
	lastState, _ := state.LoadState()
	if lastState != nil && lastState.Status == "running" {
		log.Printf("RECOVERY: Detected interrupted job %s. Reporting to server...", lastState.JobID)
	}

	// Auth: load existing tokens
	var tokens *TokenPair
	tokens, _ = loadTokens()

	if tokens == nil {
		log.Println("No tokens found; enrolling over the verified HTTPS control channel.")
		tokens, err = registerAgent(ctx, controlClient, nodeID, os.Getenv)
		if err != nil {
			return err
		}
		if err := saveTokens(*tokens); err != nil {
			return fmt.Errorf("persist agent tokens: %w", err)
		}
	}

	connectAndRun := func() error {
		header := http.Header{}
		header.Set("Authorization", "Bearer "+tokens.AccessToken)

		log.Printf("Connecting to verified control WebSocket")
		conn, resp, err := controlClient.DialWebSocket(ctx, "/ws", header)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusUnauthorized {
				log.Println("Unauthorized. Attempting token refresh...")
				newTokens, err := refreshTokens(ctx, controlClient, nodeID, tokens.RefreshToken)
				if err == nil {
					tokens = newTokens
					_ = saveTokens(*tokens)
					return fmt.Errorf("retry")
				}
			}
			log.Printf("WebSocket dial error: %v. Falling back to HTTP Long-Polling...", err)
			startHTTPPolling(ctx, controlClient, nodeID, pm, tokens)
			return err
		}
		defer func() { _ = conn.Close() }()

		go runHeartbeat(ctx, conn, nodeID, pm)

		for {
			var wsMsg WSMessage
			err := conn.ReadJSON(&wsMsg)
			if err != nil {
				return err
			}

			if wsMsg.Type == MsgTypeCommand {
				var cmd CommandPayload
				if err := json.Unmarshal(wsMsg.Payload, &cmd); err != nil {
					log.Printf("Command unmarshal error: %v", err)
					continue
				}
				log.Printf("Received command: %s (Job: %s)", cmd.Action, cmd.ID)
				go executeCommand(ctx, pm, cmd)
			}
		}
	}

	for {
		err := connectAndRun()
		if err != nil {
			if err.Error() == "retry" {
				continue
			}
			log.Printf("Connection error: %v. Retrying in 30s...", err)

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(30 * time.Second):
			}
		}
	}
}

func runHeartbeat(ctx context.Context, conn *websocket.Conn, nodeID string, pm updater.PackageManager) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hostname, _ := os.Hostname()
			rebootNeeded := pm.RebootRequired()

			if rebootNeeded && runtime.GOOS != "windows" {
				_ = exec.Command("wall", "Patchli: System reboot is required to finish updates.").Run()
			}

			payload := HeartbeatPayload{
				MacAddress:   nodeID,
				Hostname:     hostname,
				OS:           runtime.GOOS,
				OSVersion:    "unknown",
				Kernel:       "unknown",
				RebootNeeded: rebootNeeded,
			}
			data, err := json.Marshal(payload)
			if err != nil {
				continue
			}

			msg := WSMessage{Type: MsgTypeHeartbeat, Payload: data}
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("Heartbeat error: %v", err)
				return
			}
		}
	}
}

func startHTTPPolling(ctx context.Context, controlClient *control.Client, nodeID string, pm updater.PackageManager, tokens *TokenPair) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		hostname, _ := os.Hostname()
		hb := HeartbeatPayload{
			MacAddress:   nodeID,
			Hostname:     hostname,
			OS:           runtime.GOOS,
			OSVersion:    "unknown",
			Kernel:       "unknown",
			RebootNeeded: pm.RebootRequired(),
		}
		data, err := json.Marshal(hb)
		if err != nil {
			log.Printf("Failed to encode heartbeat: %v", err)
			return
		}

		req, err := controlClient.NewRequest(ctx, http.MethodPost, "/api/v1/poll", bytes.NewReader(data))
		if err != nil {
			log.Printf("Failed to create poll request: %v", err)
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if tokens != nil {
			req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		}

		resp, err := controlClient.Do(req)
		if err != nil {
			log.Printf("HTTP Poll error: %v", err)
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var cmd CommandPayload
			if err := json.NewDecoder(control.LimitedBody(resp.Body)).Decode(&cmd); err == nil && cmd.Action != "" {
				go executeCommand(ctx, pm, cmd)
			}
		} else if resp.StatusCode == http.StatusUnauthorized && tokens != nil {
			log.Println("Poll Unauthorized. Attempting token refresh...")
			newTokens, err := refreshTokens(ctx, controlClient, nodeID, tokens.RefreshToken)
			if err == nil {
				tokens = newTokens
				_ = saveTokens(*tokens)
			}
		}

		_ = resp.Body.Close()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func executeCommand(ctx context.Context, pm updater.PackageManager, cmd CommandPayload) {
	var res updater.UpdateResult
	var err error
	if (cmd.PrePatchScript != "" || cmd.PostPatchScript != "" || cmd.HealthCheckCommand != "") && os.Getenv("ALLOW_REMOTE_SCRIPTS") != "true" {
		log.Printf("Job %s rejected: remote scripts require ALLOW_REMOTE_SCRIPTS=true", cmd.ID)
		return
	}

	if cmd.Action == "apply_updates" {
		if err := pm.PreFlightCheck(ctx); err != nil {
			log.Printf("PreFlight Check Failed for Job %s: %v", cmd.ID, err)
			return
		}

		if cmd.PrePatchScript != "" {
			log.Printf("Executing Pre-Patch Script...")
			out, err := shellCommandContext(ctx, cmd.PrePatchScript).CombinedOutput()
			if err != nil {
				log.Printf("Pre-Patch Script Failed: %v, Output: %s", err, string(out))
				return
			}
		}

		if err := state.SaveState(state.State{
			JobID:  cmd.ID,
			Action: cmd.Action,
			Status: "running",
		}); err != nil {
			log.Printf("Failed to save state: %v", err)
		}
		defer func() {
			_ = state.ClearState()
		}()
	}

	switch cmd.Action {
	case "check_updates":
		res, err = pm.CheckUpdates(ctx)
	case "apply_updates":
		res, err = pm.ApplyUpdates(ctx, cmd.Packages)
	case "cleanup":
		err = pm.Cleanup(ctx)
		res = updater.UpdateResult{Success: err == nil, Error: err}
	case "self_destruct":
		err = updater.SelfDestruct()
		res = updater.UpdateResult{Success: err == nil, Error: err}
	case "update_agent":
		log.Printf("Auto-updating agent...")
		res = performSecureAgentUpdateFunc(ctx)
	case "chaos_restart":
		log.Printf("CHAOS MONKEY: Restarting agent...")
		os.Exit(1)
	default:
		log.Printf("Unknown action: %s", cmd.Action)
		return
	}

	if cmd.Action == "apply_updates" && err == nil {
		if cmd.PostPatchScript != "" {
			log.Printf("Executing Post-Patch Script...")
			out, execErr := shellCommandContext(ctx, cmd.PostPatchScript).CombinedOutput()
			if execErr != nil {
				log.Printf("Post-Patch Script Failed: %v, Output: %s", execErr, string(out))
				err = execErr
				res.Success = false
			}
		}

		if err == nil && cmd.HealthCheckCommand != "" {
			log.Printf("Executing Health Check Command...")
			out, execErr := shellCommandContext(ctx, cmd.HealthCheckCommand).CombinedOutput()
			if execErr != nil {
				log.Printf("Health Check Failed: %v, Output: %s", execErr, string(out))
				err = execErr
				res.Success = false
			}
		}
	}

	log.Printf("Job %s finished. Success: %v. Error: %v", cmd.ID, res.Success, err)
}

var performSecureAgentUpdateFunc = func(ctx context.Context) updater.UpdateResult {
	controlClient, err := control.FromEnvironment(os.Getenv)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	req, err := controlClient.NewRequest(ctx, http.MethodGet, "/download/agent", nil)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}

	resp, err := controlClient.Do(req)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("HTTP %d during agent download", resp.StatusCode)}
	}

	tmpFile, err := os.CreateTemp("", "patchli-agent-*")
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, 100<<20+1))
	if err != nil {
		_ = tmpFile.Close()
		return updater.UpdateResult{Success: false, Error: err}
	}
	if written > 100<<20 {
		_ = tmpFile.Close()
		return updater.UpdateResult{Success: false, Error: errors.New("agent download exceeds 100 MiB limit")}
	}
	if err := tmpFile.Close(); err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}

	// Download the signature file before verification
	sigReq, err := controlClient.NewRequest(ctx, http.MethodGet, "/download/agent.sig", nil)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("failed to create signature download request: %v", err)}
	}
	sigResp, err := controlClient.Do(sigReq)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("failed to download signature: %v", err)}
	}
	defer func() { _ = sigResp.Body.Close() }()

	if sigResp.StatusCode != http.StatusOK {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("HTTP %d during signature download", sigResp.StatusCode)}
	}

	sigFile, err := os.CreateTemp("", "patchli-agent-sig-*")
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	sigName := sigFile.Name()
	defer func() {
		_ = os.Remove(sigName)
	}()

	if _, err := io.Copy(sigFile, io.LimitReader(sigResp.Body, 1<<20)); err != nil {
		_ = sigFile.Close()
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("failed to write signature file: %v", err)}
	}
	if err := sigFile.Close(); err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}

	// Rename the signature file to match what verifySignature expects
	sigDest := tmpName + ".sig"
	if err := os.Rename(sigName, sigDest); err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("failed to rename signature file: %v", err)}
	}
	defer func() {
		_ = os.Remove(sigDest)
	}()

	if err := verifySignature(tmpName); err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("signature verification failed: %v", err)}
	}

	// #nosec G302 -- the verified downloaded agent must be executable before atomic replacement.
	if err := os.Chmod(tmpName, 0o755); err != nil {
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

	out, err := restartAgent(ctx)
	return updater.UpdateResult{Success: err == nil, Output: string(out), Error: err}
}

func verifySignature(filePath string) error {
	if TrustedPubKey == "" {
		return fmt.Errorf("TRUSTED_PUB_KEY not set: refusing to verify signature")
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(TrustedPubKey)
	if err != nil {
		return fmt.Errorf("failed to decode trusted public key: %v", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size")
	}

	sigPath := filePath + ".sig"
	// #nosec G304 -- sigPath is derived only from the agent's internally-created temporary file.
	sigBase64, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("failed to read signature file %s: %v", sigPath, err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(string(sigBase64))
	if err != nil {
		return fmt.Errorf("failed to decode signature: %v", err)
	}

	// #nosec G304 -- filePath is the agent's internally-created temporary update file.
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read binary: %v", err)
	}

	if !ed25519.Verify(pubKeyBytes, data, sigBytes) {
		return fmt.Errorf("cryptographic signature verification failed")
	}
	return nil
}

func restartAgent(ctx context.Context) ([]byte, error) {
	if runtime.GOOS == "windows" {
		psPath, err := exec.LookPath("powershell.exe")
		if err != nil {
			return nil, fmt.Errorf("failed to find powershell.exe: %v", err)
		}
		// #nosec G204 -- psPath is resolved by exec.LookPath and the PowerShell program text is constant.
		helper := exec.Command(psPath, "-Command", "Start-Sleep -Seconds 2; Start-Service -Name patchli-agent")
		_ = helper.Start()
		// #nosec G204 -- psPath is resolved by exec.LookPath and the PowerShell program text is constant.
		_ = exec.CommandContext(ctx, psPath, "-Command", "Stop-Service -Name patchli-agent").Run()
		return []byte("Restarting via powershell.exe helper"), nil
	}
	if _, err := os.Stat("/run/openrc"); err == nil {
		return exec.CommandContext(ctx, "rc-service", "patchli-agent", "restart").CombinedOutput()
	}
	return exec.CommandContext(ctx, "systemctl", "restart", "patchli-agent").CombinedOutput()
}
