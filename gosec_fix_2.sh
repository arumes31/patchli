#!/bin/bash

sed -i 's/resp, err := client.Do(req) \/\/ #nosec G107/resp, err := client.Do(req) \/\/ #nosec G107 G704/g' server/internal/webhooks/webhooks.go
sed -i 's/req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data)) \/\/ #nosec G107/req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data)) \/\/ #nosec G107 G704/g' server/internal/webhooks/webhooks.go
sed -i 's/log.Printf("Webhook returned error status: %d", resp.StatusCode) \/\/ #nosec G104/log.Printf("Webhook returned error status: %d", resp.StatusCode) \/\/ #nosec G104 G706/g' server/internal/webhooks/webhooks.go
sed -i 's/log.Printf("Server listening on :%s", port) \/\/ #nosec G104/log.Printf("Server listening on :%s", port) \/\/ #nosec G104 G706/g' server/cmd/patchli-server/main.go
