## 2025-02-14 - SSRF via Taint Analysis (GoSec False Positive Mitigation)
**Vulnerability:** Gosec flagged `G704` for HTTP client requests in `server/internal/webhooks/webhooks.go` where target URLs were derived from environment variables.
**Learning:** Adding validation logic like parsing IPs without DNS resolution introduces bypasses. Since webhook URLs originate strictly from trusted environment variables and aren't user-controllable, complex validation represents security theater.
**Prevention:** Apply explicit `// #nosec G704 -- Trusted URLs from environment configuration` to correctly suppress the scanner when URLs are sourced directly from trusted environment variables, preventing false positives without adding ineffective logic.
