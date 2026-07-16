## 2024-07-16 - Prevent XSS in Setup UI via Host header injection
**Vulnerability:** Cross-Site Scripting (XSS) vulnerability was present due to the use of `text/template` for rendering the `setup.html` page, which included the potentially user-controlled `r.Host` value.
**Learning:** `text/template` does not perform automatic contextual escaping, meaning that unescaped dynamic values can lead to XSS if an attacker can manipulate the input (e.g., via a forged Host header).
**Prevention:** Always use `html/template` instead of `text/template` when rendering HTML content in Go. `html/template` provides automatic contextual escaping to prevent script injection and XSS.
