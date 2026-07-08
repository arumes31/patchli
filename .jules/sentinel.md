## 2025-02-28 - [Critical: XSS via text/template]
**Vulnerability:** XSS vulnerability through contextually unsafe rendering of HTML templates. The `ServeSetupUI` handler was using `text/template` instead of `html/template`.
**Learning:** `text/template` blindly interpolates data strings into the template text without any contextual escaping, meaning user-controlled variables (like `r.Host` which derives from HTTP headers) can easily inject arbitrary JavaScript/HTML into the page leading to an XSS vulnerability.
**Prevention:** In Go web applications, always use `html/template` instead of `text/template` when rendering HTML content to ensure automatic contextual escaping of all injected variables.
