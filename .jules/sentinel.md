## 2024-05-16 - Use html/template to prevent XSS
**Vulnerability:** The setup page rendering used `text/template`, leaving it vulnerable to XSS if the dynamically injected `BaseURL` or other user-controlled variables were malicious.
**Learning:** In Go, always use `html/template` instead of `text/template` when rendering HTML. The `html/template` package provides context-aware escaping automatically, whereas `text/template` does not.
**Prevention:** Avoid `text/template` for any output that will be processed by a browser as HTML. Standardize on `html/template`.
