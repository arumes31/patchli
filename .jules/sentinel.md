## 2026-07-24 - Prevent XSS in HTML template rendering
**Vulnerability:** XSS vulnerability in setup.html due to `text/template` use.
**Learning:** In Go applications, always use `html/template` instead of `text/template` for rendering HTML content to ensure automatic contextual escaping and prevent Cross-Site Scripting (XSS) vulnerabilities (e.g., when rendering `r.Host` or user-controlled URLs).
**Prevention:** Use `html/template` for all HTML files.
