## 2025-07-11 - [XSS via Host Header in text/template]
**Vulnerability:** The setup page used `text/template` and reflected the user-provided HTTP Host header `r.Host` directly into the page.
**Learning:** `text/template` does not contextually escape HTML strings. If `r.Host` is controlled by a malicious user (e.g. by modifying Host headers or DNS rebinding), it could inject malicious scripts.
**Prevention:** Always use `html/template` when outputting HTML content, as it contextually escapes variables.
