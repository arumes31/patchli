## 2025-02-24 - [Fix XSS in setup UI]
**Vulnerability:** XSS vulnerability by rendering user-controlled fields using `text/template` instead of `html/template` in `server/internal/api/handlers.go`.
**Learning:** In Go applications, always use `html/template` instead of `text/template` for rendering HTML content to ensure automatic contextual escaping and prevent Cross-Site Scripting (XSS) vulnerabilities (e.g., when rendering `r.Host` or user-controlled URLs).
**Prevention:** Verify the template package being imported when rendering HTML. If it is `text/template`, change it to `html/template` to enable contextual escaping and prevent XSS.
