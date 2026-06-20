## 2025-02-14 - Use html/template to prevent XSS
**Vulnerability:** `text/template` was used to render HTML, leading to potential Cross-Site Scripting (XSS) via unescaped host headers (`r.Host`).
**Learning:** `text/template` does not contextually encode values rendered inside HTML templates.
**Prevention:** Always use `html/template` instead of `text/template` for rendering HTML content to ensure automatic contextual escaping and prevent XSS vulnerabilities.
