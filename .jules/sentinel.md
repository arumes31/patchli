## 2024-05-30 - Prevent XSS in HTML rendering
**Vulnerability:** The server used `text/template` for rendering the HTML setup page, which doesn't automatically escape context. When it renders data like `BaseURL` containing user-controlled values (such as `r.Host` from the request), it introduces an XSS risk.
**Learning:** In Go applications, HTML templates should always be rendered using `html/template`, never `text/template`, to ensure automatic contextual escaping.
**Prevention:** Use `html/template` by default when outputting HTML content, and specifically audit any usage of `text/template` in the codebase.
