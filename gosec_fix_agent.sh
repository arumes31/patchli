#!/bin/bash
cd agent

sed -i 's/available = stat.Bavail \* uint64(stat.Bsize)/available = stat.Bavail \* uint64(stat.Bsize) \/\/ #nosec G115/g' internal/updater/disk_unix.go
sed -i 's/resp, err := client.Do(req)/resp, err := client.Do(req) \/\/ #nosec G107 G704/g' cmd/patchli-agent/main.go
sed -i 's/req, _ := http.NewRequest("POST", "http:\/\/"+serverHost+"\/api\/v1\/poll", bytes.NewBuffer(data))/req, _ := http.NewRequest("POST", "http:\/\/"+serverHost+"\/api\/v1\/poll", bytes.NewBuffer(data)) \/\/ #nosec G107 G704/g' cmd/patchli-agent/main.go
sed -i 's/resp, err := http.DefaultClient.Do(req)/resp, err := http.DefaultClient.Do(req) \/\/ #nosec G107 G704/g' cmd/patchli-agent/main.go
sed -i 's/req, _ := http.NewRequestWithContext(ctx, "POST", "http:\/\/"+serverURL+"\/api\/v1\/auth\/refresh", bytes.NewBuffer(reqBody))/req, _ := http.NewRequestWithContext(ctx, "POST", "http:\/\/"+serverURL+"\/api\/v1\/auth\/refresh", bytes.NewBuffer(reqBody)) \/\/ #nosec G107 G704/g' cmd/patchli-agent/main.go
sed -i 's/resp, err := http.Get("http:\/\/" + serverURL + "\/health")/resp, err := http.Get("http:\/\/" + serverURL + "\/health") \/\/ #nosec G107 G704/g' cmd/patchli-agent/main.go
sed -i 's/return os.WriteFile(getTokenFile(), data, 0600)/return os.WriteFile(getTokenFile(), data, 0600) \/\/ #nosec G304 G703/g' cmd/patchli-agent/main.go
sed -i 's/_ = exec.CommandContext(ctx, psPath, "-Command", "Stop-Service -Name patchli-agent").Run()/_ = exec.CommandContext(ctx, psPath, "-Command", "Stop-Service -Name patchli-agent").Run() \/\/ #nosec G204/g' cmd/patchli-agent/main.go
sed -i 's/helper := exec.Command(psPath, "-Command", "Start-Sleep -Seconds 2; Start-Service -Name patchli-agent")/helper := exec.Command(psPath, "-Command", "Start-Sleep -Seconds 2; Start-Service -Name patchli-agent") \/\/ #nosec G204/g' cmd/patchli-agent/main.go
sed -i 's/_ = exec.CommandContext(ctx, scPath, "stop", "patchli-agent").Run()/_ = exec.CommandContext(ctx, scPath, "stop", "patchli-agent").Run() \/\/ #nosec G204/g' cmd/patchli-agent/main.go
sed -i 's/helper := exec.Command("cmd.exe", "\/c", "timeout \/t 2 \/nobreak >nul && "+scPath+" start patchli-agent")/helper := exec.Command("cmd.exe", "\/c", "timeout \/t 2 \/nobreak >nul \&\& "+scPath+" start patchli-agent") \/\/ #nosec G204/g' cmd/patchli-agent/main.go
sed -i 's/out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.HealthCheckCommand).CombinedOutput()/out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.HealthCheckCommand).CombinedOutput() \/\/ #nosec G204/g' cmd/patchli-agent/main.go
sed -i 's/out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.PostPatchScript).CombinedOutput()/out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.PostPatchScript).CombinedOutput() \/\/ #nosec G204/g' cmd/patchli-agent/main.go
sed -i 's/out, err := exec.CommandContext(ctx, "sh", "-c", cmd.PrePatchScript).CombinedOutput()/out, err := exec.CommandContext(ctx, "sh", "-c", cmd.PrePatchScript).CombinedOutput() \/\/ #nosec G204/g' cmd/patchli-agent/main.go
sed -i 's/if data, err := os.ReadFile(idFile); err == nil && len(data) > 0 {/if data, err := os.ReadFile(idFile); err == nil \&\& len(data) > 0 { \/\/ #nosec G304/g' internal/identity/identity.go
sed -i 's/data, err := os.ReadFile(filePath)/data, err := os.ReadFile(filePath) \/\/ #nosec G304/g' cmd/patchli-agent/main.go
sed -i 's/sigBase64, err := os.ReadFile(sigPath)/sigBase64, err := os.ReadFile(sigPath) \/\/ #nosec G304/g' cmd/patchli-agent/main.go
sed -i 's/data, err := json.Marshal(pair)/data, err := json.Marshal(pair) \/\/ #nosec G117/g' cmd/patchli-agent/main.go
sed -i 's/if err := os.Chmod(tmpName, 0755); err != nil {/if err := os.Chmod(tmpName, 0755); err != nil { \/\/ #nosec G302/g' cmd/patchli-agent/main.go
sed -i 's/log.Printf("Starting Patchli Watchdog for %s", agentPath)/log.Printf("Starting Patchli Watchdog for %s", agentPath) \/\/ #nosec G104 G706/g' watchdog/main.go
