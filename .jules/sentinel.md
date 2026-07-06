## 2024-05-18 - [XSS via text/template]
**Vulnerability:** The application was using `text/template` to render the setup UI, passing in `r.Host` which is user-controlled (via the `Host` header or `X-Forwarded-Host`). This creates a Cross-Site Scripting (XSS) vulnerability.
**Learning:** In Go, `text/template` does not perform automatic contextual escaping. Any string passed into the template is rendered exactly as-is.
**Prevention:** Always use `html/template` when rendering HTML content in Go. It automatically escapes data based on its context (HTML, JS, CSS, URI, etc.), preventing script injection.
