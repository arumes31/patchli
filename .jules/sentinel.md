## 2025-02-27 - [Cross-Site Scripting (XSS) via `text/template` in HTML responses]
**Vulnerability:** The application was using the `text/template` package to render `setup.html` in `server/internal/api/handlers.go`. Because `text/template` does not contextually escape HTML, passing potentially user-influenced variables (like the `{{.BaseURL}}` constructed from request headers) exposed the UI to XSS.
**Learning:** `text/template` simply injects text without encoding it for the target language context, which is unsafe when generating HTML.
**Prevention:** Always use `html/template` when rendering HTML content in Go. The interface is identical, but `html/template` provides automatic, contextual auto-escaping (preventing injection of raw script tags and other malicious inputs).
