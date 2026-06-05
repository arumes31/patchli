## 2025-02-27 - Setup script injection
**Vulnerability:** The setup endpoints generated Linux, Alpine, and Windows shell scripts using `fmt.Sprintf` with double quotes `%s` for string interpolation from URL query parameters without escaping. This creates a critical command injection vulnerability during agent installation.
**Learning:** Even though the parameters (like `group`) may seem benign, when outputting data directly into a shell context (Bash or PowerShell scripts), they must be properly escaped. Double quotes allow string escape sequences or variable interpolation to run arbitrary commands.
**Prevention:** Use single quotes to encapsulate variables in scripts and apply appropriate escaping mechanisms, such as `'\''` for Bash and `''` for PowerShell.

## 2025-02-27 - Logging unsanitized input
**Vulnerability:** Port variable was retrieved from the environment (`os.Getenv("PORT")`) and logged directly using `log.Printf("Server listening on :%s", port)`. This is a classic log injection vulnerability, where an attacker who can control the environment variable could insert newlines to spoof log entries. A similar issue existed with errors logged from webhook invocations.
**Learning:** Even though the source of the data might seem trusted (like an environment variable or an error returned by the standard library), it should be sanitized before being written to logs if it could ever contain user-influenced data (e.g. carriage returns or line feeds) to prevent log forging.
**Prevention:** Sanitize inputs by stripping out or escaping newline (`\n`) and carriage return (`\r`) characters before logging them. `strings.ReplaceAll(strings.ReplaceAll(port, "\n", ""), "\r", "")` is a simple approach to prevent log injection.
