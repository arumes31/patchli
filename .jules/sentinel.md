## 2024-07-01 - Use html/template to prevent XSS in Go handlers
**Vulnerability:** The codebase was using `text/template` to render HTML templates containing dynamic user input (`r.Host`).
**Learning:** `text/template` doesn't automatically escape output depending on context, making the application vulnerable to XSS attacks when user-controlled data is rendered into an HTML document.
**Prevention:** Always use `html/template` instead of `text/template` when rendering HTML content in Go. The `html/template` package understands HTML context and automatically escapes variables securely.
