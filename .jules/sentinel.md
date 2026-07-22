## 2026-07-22 - [XSS vulnerability fix]
**Vulnerability:** Using text/template for rendering HTML templates in Go applications.
**Learning:** text/template does not provide automatic contextual escaping, leading to Cross-Site Scripting (XSS) vulnerabilities if dynamic content is rendered.
**Prevention:** Always use html/template when rendering HTML content.
