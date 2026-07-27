## 2026-07-27 - XSS in Go Templates
**Vulnerability:** `text/template` was used to render `setup.html` where user-controlled input like `r.Host` is displayed, which doesn't automatically escape context allowing for Cross-Site Scripting (XSS).
**Learning:** `html/template` provides the exact same interface as `text/template` but performs automatic contextual escaping of data being injected into the template to prevent XSS. It's the standard for any HTML rendering.
**Prevention:** Always use `html/template` when rendering any HTML content in Go applications, never `text/template`.
