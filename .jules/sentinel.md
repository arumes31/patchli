## 2025-02-14 - text/template XSS in HTML output
**Vulnerability:** The `/api/v1/setup` endpoint in `server/internal/api/handlers.go` used `text/template` instead of `html/template` to render an HTML page (`setup.html`) containing the dynamically generated `r.Host` parameter as a `BaseURL`. `text/template` doesn't automatically escape content appropriately for HTML, which could allow XSS if an attacker controls the `X-Forwarded-Host` or `Host` headers.
**Learning:** Always use `html/template` when serving HTML templates in Go.
**Prevention:** Make it a rule to check for imports of `text/template` in any web service rendering HTML context.
