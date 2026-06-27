## 2025-06-27 - [High] Fix Reflected XSS in Setup UI
**Vulnerability:** Reflected Cross-Site Scripting (XSS) due to use of `text/template` instead of `html/template`.
**Learning:** `text/template` does not auto-escape values during templating. A user-provided value (like `r.Host`) can inject arbitrary scripts in the generated HTML. Go standard library recommends `html/template` when outputting HTML content.
**Prevention:** Always use `html/template` when rendering dynamic HTML with untrusted inputs.
