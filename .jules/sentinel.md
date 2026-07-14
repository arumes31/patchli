## 2024-07-14 - text/template vs html/template XSS Risk
**Vulnerability:** XSS vulnerability in `ServeSetupUI` caused by using `text/template` to render `r.Host` directly into an HTML page.
**Learning:** `text/template` in Go does not auto-escape input when rendering templates, making it unsafe for generating HTML from user-controllable input (like the `Host` header).
**Prevention:** Always use `html/template` when rendering HTML content in Go applications to ensure automatic contextual escaping.
