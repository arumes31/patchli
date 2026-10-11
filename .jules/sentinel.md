## 2024-07-28 - Cross-Site Scripting (XSS) in HTML templates
**Vulnerability:** The application was using `text/template` instead of `html/template` to render HTML templates. This could allow for XSS vulnerabilities because `text/template` does not automatically escape data context strings (like `r.Host`) when rendering templates, meaning malicious input could be injected and executed by the browser.
**Learning:** In Go applications, rendering HTML content should always use `html/template` instead of `text/template` because it provides automatic contextual escaping to protect against XSS attacks.
**Prevention:** Always use `html/template` to generate HTML, regardless of whether the input seems safe or controlled.
