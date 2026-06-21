## 2024-06-21 - XSS in HTML Template Rendering
**Vulnerability:** Used `text/template` instead of `html/template` to render an HTML page (`setup.html`) containing user-controlled input (`r.Host`).
**Learning:** `text/template` does not perform contextual escaping, allowing an attacker to inject malicious scripts (XSS) by manipulating the `Host` header.
**Prevention:** Always use `html/template` for HTML output to ensure variables are automatically and safely escaped based on their context.
