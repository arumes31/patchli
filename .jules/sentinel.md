## 2025-02-05 - Fix XSS Vulnerability via template package
**Vulnerability:** The application uses `text/template` instead of `html/template` to render HTML templates. This exposes the application to XSS vulnerabilities, particularly because the `baseURL` generated from `r.Host` is rendered in `ServeSetupUI`, which could be manipulated by a user supplying a malicious `Host` header.
**Learning:** `text/template` does not perform contextual escaping, so user-controlled strings (like the `r.Host` header) are rendered raw into the HTML output. Go's `html/template` provides automatic, context-aware escaping specifically designed to prevent XSS.
**Prevention:** Always use `html/template` to parse and execute HTML files instead of `text/template`.
