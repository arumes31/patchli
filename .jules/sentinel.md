## 2026-08-03 - Prevent XSS in Go Templates
**Vulnerability:** Used text/template for rendering HTML responses (setup UI) containing user-controlled values like r.Host in BaseURL. This exposes the app to Cross-Site Scripting (XSS) attacks because text/template does not perform contextual escaping.
**Learning:** In Go, text/template only substitutes text without escaping. When generating HTML, html/template should ALWAYS be used to ensure values are automatically and contextually escaped (e.g. escaping HTML tags, JS strings, or URLs).
**Prevention:** Always use "html/template" when generating any response with a Content-Type of text/html or when sending data that a browser will parse as HTML.
