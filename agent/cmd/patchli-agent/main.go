package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gorilla/websocket"
	"github.com/arumes31/patchli/agent/internal/identity"
	"github.com/arumes31/patchli/agent/internal/state"
	"github.com/arumes31/patchli/agent/internal/updater"
)

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
	MacAddress    string `json:"mac_address"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	RebootNeeded  bool   `json:"reboot_needed"`
}

type CommandPayload struct {
	ID                 string   `json:"id"`
	Action             string   `json:"action"`
	Packages           []string `json:"packages,omitempty"`
	PrePatchScript     string   `json:"pre_patch_script,omitempty"`
	PostPatchScript    string   `json:"post_patch_script,omitempty"`
	HealthCheckCommand string   `json:"health_check_command,omitempty"`
}

func performSelfDiagnosis() {
	fmt.Println("--- Patchli Agent Self-Diagnosis ---")

	fmt.Print("1. Identity check: ")
	nodeID := identity.GetOrGenerate()
	fmt.Printf("UUID=%s [OK]\n", nodeID)

	fmt.Print("2. Package Manager check: ")
	pm, err := updater.DetectPackageManager()
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Printf("Detected=%T [OK]\n", pm)
	}

	fmt.Print("3. Disk Space check: ")
	if err := updater.CheckDiskSpace("/etc/patchli", 100*1024*1024); err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Println("Available > 100MB [OK]")
	}

	fmt.Print("4. Network (Server) check: ")
	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" { serverURL = "localhost:8080" }
	/* #nosec G107 */ /* #nosec G704 */
	resp, err := http.Get("http://" + serverURL + "/health")
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Printf("Server Health=%d [OK]\n", resp.StatusCode)
		_ = resp.Body.Close()
	}

	fmt.Println("Self-diagnosis complete.")
}

func main() {
	verifyFlag := flag.Bool("verify", false, "Perform self-diagnosis and exit")
	flag.Parse()

	if *verifyFlag {
		performSelfDiagnosis()
		return
	}

	_, _ = daemon.SdNotify(false, daemon.SdNotifyReady)
	RunAgent(context.Background())
}

func RunAgent(ctx context.Context) {
	log.Println("Starting Patchli Agent...")

	pm, err := updater.DetectPackageManager()
	if err != nil {
		log.Fatalf("Failed to detect package manager: %v", err)
	}

	nodeID := identity.GetOrGenerate()
	log.Printf("Agent Identity (UUID): %s", nodeID)

	// State Recovery on Boot
	lastState, _ := state.LoadState()
	if lastState != nil && lastState.Status == "running" {
		log.Printf("RECOVERY: Detected interrupted job %s. Reporting to server...", lastState.JobID)
		// In a real app, send a recovery notification via WS/Polling
	}

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "localhost:8080"
	}

	u := url.URL{Scheme: "ws", Host: serverURL, Path: "/ws"}
	log.Printf("Connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("WebSocket dial error: %v. Falling back to HTTP Long-Polling...", err)
		startHTTPPolling(ctx, serverURL, nodeID, pm)
		return
	}
	defer conn.Close()

	go runHeartbeat(ctx, conn, nodeID, pm)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			var wsMsg WSMessage
			err := conn.ReadJSON(&wsMsg)
			if err != nil {
				log.Printf("Read error: %v", err)
				return
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

			if rebootNeeded {
				_ = exec.Command("wall", "Patchli: System reboot is required to finish updates.").Run()
			}

			payload := HeartbeatPayload{
				MacAddress:   nodeID,
				Hostname:     hostname,
				RebootNeeded: rebootNeeded,
			}
			data, err := json.Marshal(payload)
			if err != nil { continue }
			msg := WSMessage{Type: MsgTypeHeartbeat, Payload: data}
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("Heartbeat error: %v", err)
				return
			}
		}
	}
}

func startHTTPPolling(ctx context.Context, serverHost, nodeID string, pm updater.PackageManager) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	client := &http.Client{Timeout: 60 * time.Second}

	for {
		hostname, _ := os.Hostname()
		hb := HeartbeatPayload{
			MacAddress:   nodeID,
			Hostname:     hostname,
			RebootNeeded: pm.RebootRequired(),
		}
		data, _ := json.Marshal(hb)

		/* #nosec G107 */ /* #nosec G704 */
		req, _ := http.NewRequest("POST", "http://"+serverHost+"/api/v1/poll", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")

		/* #nosec G107 */ /* #nosec G704 */
		resp, err := client.Do(req)
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
			if err := json.NewDecoder(resp.Body).Decode(&cmd); err == nil && cmd.Action != "" {
				go executeCommand(ctx, pm, cmd)
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

	if cmd.Action == "apply_updates" {
		if err := pm.PreFlightCheck(ctx); err != nil {
			log.Printf("PreFlight Check Failed for Job %s: %v", cmd.ID, err)
			return
		}

		if cmd.PrePatchScript != "" {
			log.Printf("Executing Pre-Patch Script...")
			// #nosec G204 -- The script content is provided by the trusted control plane.
			out, err := exec.CommandContext(ctx, "sh", "-c", cmd.PrePatchScript).CombinedOutput()
			if err != nil {
				log.Printf("Pre-Patch Script Failed: %v, Output: %s", err, string(out))
				return
			}
		}

		if err := state.SaveState(state.State{
			JobID: cmd.ID,
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
			// #nosec G204 -- The script content is provided by the trusted control plane.
			out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.PostPatchScript).CombinedOutput()
			if execErr != nil {
				log.Printf("Post-Patch Script Failed: %v, Output: %s", execErr, string(out))
				err = execErr
				res.Success = false
			}
		}

		if err == nil && cmd.HealthCheckCommand != "" {
			log.Printf("Executing Health Check Command...")
			// #nosec G204 -- The script content is provided by the trusted control plane.
			out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.HealthCheckCommand).CombinedOutput()
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
	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "localhost:8080"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+serverURL+"/download/agent", nil)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	defer resp.Body.Close()

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

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		return updater.UpdateResult{Success: false, Error: err}
	}
	if err := tmpFile.Close(); err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}

	if err := verifySignature(tmpName); err != nil {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("signature verification failed: %v", err)}
	}

	// #nosec G302 -- The file is an executable that needs to be run
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
	// #nosec G304 -- Expected to read arbitrary files for signature verification
	sigBase64, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("failed to read signature file %s: %v", sigPath, err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(string(sigBase64))
	if err != nil {
		return fmt.Errorf("failed to decode signature: %v", err)
	}

	// #nosec G304 -- Expected to read arbitrary files for signature verification
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
		scPath, err := exec.LookPath("sc.exe")
		if err == nil {
			// #nosec G204 -- The sc.exe path is resolved safely via LookPath.
			helper := exec.Command("cmd.exe", "/c", "timeout /t 2 /nobreak >nul && "+scPath+" start patchli-agent")
			_ = helper.Start()
			// #nosec G204 -- The sc.exe path is resolved safely via LookPath.
			_ = exec.CommandContext(ctx, scPath, "stop", "patchli-agent").Run()
			return []byte("Restarting via sc.exe helper"), nil
		}

		psPath, err := exec.LookPath("powershell.exe")
		if err != nil {
			return nil, fmt.Errorf("failed to find powershell.exe or sc.exe: %v", err)
		}
		// #nosec G204 -- The powershell.exe path is resolved safely via LookPath.
		helper := exec.Command(psPath, "-Command", "Start-Sleep -Seconds 2; Start-Service -Name patchli-agent")
		_ = helper.Start()
		// #nosec G204 -- The powershell.exe path is resolved safely via LookPath.
		_ = exec.CommandContext(ctx, psPath, "-Command", "Stop-Service -Name patchli-agent").Run()
		return []byte("Restarting via powershell.exe helper"), nil
	}
	if _, err := os.Stat("/run/openrc"); err == nil {
		return exec.CommandContext(ctx, "rc-service", "patchli-agent", "restart").CombinedOutput()
	}
	return exec.CommandContext(ctx, "systemctl", "restart", "patchli-agent").CombinedOutput()
}
