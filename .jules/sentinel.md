## 2026-06-19 - XSS in HTML Template Rendering
**Vulnerability:** XSS vulnerability due to the use of `text/template` to render HTML templates. Malicious input, such as the `Host` header (`r.Host`), can be injected into the response because `text/template` does not perform automatic contextual escaping.
**Learning:** `text/template` renders data exactly as-is, which is dangerous for HTML content where characters like `<`, `>`, and `"` must be escaped.
**Prevention:** Always use `html/template` for HTML generation to ensure contextual escaping of variables.
