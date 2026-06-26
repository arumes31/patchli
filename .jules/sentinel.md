## 2024-06-26 - [Fix XSS vulnerability in setup UI rendering]
**Vulnerability:** XSS vulnerability in setup UI rendering due to the use of `text/template` instead of `html/template`.
**Learning:** In Go applications, always use `html/template` instead of `text/template` for rendering HTML content to ensure automatic contextual escaping and prevent Cross-Site Scripting (XSS) vulnerabilities (e.g., when rendering `r.Host` or user-controlled URLs).
**Prevention:** Always verify that `html/template` is used for parsing and executing HTML files.
