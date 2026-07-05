## 2024-06-25 - [Use html/template for HTML rendering]
**Vulnerability:** XSS (Cross-Site Scripting) vulnerability when generating HTML content using `text/template`, potentially allowing malicious injection via user-controlled data like `r.Host`.
**Learning:** `text/template` does not provide contextual escaping for HTML outputs, making it unsafe for generating dynamic web pages.
**Prevention:** Always use `html/template` instead of `text/template` when rendering HTML content in Go to ensure automatic contextual escaping and mitigate XSS risks.
