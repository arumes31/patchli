## 2025-02-27 - Setup script injection
**Vulnerability:** The setup endpoints generated Linux, Alpine, and Windows shell scripts using `fmt.Sprintf` with double quotes `%s` for string interpolation from URL query parameters without escaping. This creates a critical command injection vulnerability during agent installation.
**Learning:** Even though the parameters (like `group`) may seem benign, when outputting data directly into a shell context (Bash or PowerShell scripts), they must be properly escaped. Double quotes allow string escape sequences or variable interpolation to run arbitrary commands.
**Prevention:** Use single quotes to encapsulate variables in scripts and apply appropriate escaping mechanisms, such as `'\''` for Bash and `''` for PowerShell.
