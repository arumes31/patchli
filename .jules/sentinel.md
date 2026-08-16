## 2025-02-12 - Prevent XSS in HTML Rendering
**Vulnerability:** Using `text/template` to render HTML files.
**Learning:** `text/template` does not perform automatic contextual escaping. If user-controlled data (like `r.Host` used for base URLs) is rendered into HTML or JavaScript blocks, it can lead to Cross-Site Scripting (XSS) vulnerabilities.
**Prevention:** Always use `html/template` for rendering HTML content in Go applications to ensure automatic and secure contextual escaping.
