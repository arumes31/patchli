## 2025-02-24 - Cross-Site Scripting (XSS) in setup.html
**Vulnerability:** The application used `text/template` instead of `html/template` when parsing and rendering `setup.html`. The `r.Host` parameter (which is partly user-controlled) was passed in the context and rendered in the template, creating a potential XSS vector.
**Learning:** In Go, the `html/template` package provides automatic contextual escaping to prevent script injection in HTML rendering. `text/template` does not, making it inherently dangerous when injecting dynamically constructed variables into HTML context.
**Prevention:** Always use `html/template` for rendering HTML content, particularly when user inputs or HTTP request headers are used within the template structure.
