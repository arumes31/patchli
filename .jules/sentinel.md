## 2025-02-18 - XSS in template rendering
**Vulnerability:** XSS risk due to using `text/template` instead of `html/template` when rendering `setup.html` in `server/internal/api/handlers.go`. It interpolates `r.Host` which is user-controllable.
**Learning:** In Go, `text/template` does not auto-escape values injected into templates, whereas `html/template` does. Using the wrong package makes HTML templates vulnerable to XSS.
**Prevention:** Always use `html/template` when rendering HTML.
