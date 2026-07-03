## 2024-07-03 - HTML Generation XSS via `text/template`
**Vulnerability:** Cross-Site Scripting (XSS) vulnerability due to using `text/template` instead of `html/template` when rendering `setup.html` which reflects `r.Host` back to the user.
**Learning:** `text/template` does not auto-escape values like `html/template` does. Using it to generate HTML templates with user-controlled input (like `r.Host`) allows attackers to inject malicious HTML/JS.
**Prevention:** Always use `html/template` instead of `text/template` for rendering HTML content to ensure automatic contextual escaping and prevent XSS vulnerabilities.
