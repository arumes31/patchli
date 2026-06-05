## 2025-02-27 - Setup script injection
**Vulnerability:** The setup endpoints generated Linux, Alpine, and Windows shell scripts using `fmt.Sprintf` with double quotes `%s` for string interpolation from URL query parameters without escaping. This creates a critical command injection vulnerability during agent installation.
**Learning:** Even though the parameters (like `group`) may seem benign, when outputting data directly into a shell context (Bash or PowerShell scripts), they must be properly escaped. Double quotes allow string escape sequences or variable interpolation to run arbitrary commands.
**Prevention:** Use single quotes to encapsulate variables in scripts and apply appropriate escaping mechanisms, such as `'\''` for Bash and `''` for PowerShell.

## 2025-02-27 - Logging unsanitized input
**Vulnerability:** Port variable was retrieved from the environment (`os.Getenv("PORT")`) and logged directly using `log.Printf("Server listening on :%s", port)`. This is a classic log injection vulnerability, where an attacker who can control the environment variable could insert newlines to spoof log entries. A similar issue existed with errors logged from webhook invocations.
**Learning:** Even though the source of the data might seem trusted (like an environment variable or an error returned by the standard library), it should be sanitized before being written to logs if it could ever contain user-influenced data (e.g. carriage returns or line feeds) to prevent log forging.
**Prevention:** Sanitize inputs by stripping out or escaping newline (`\n`) and carriage return (`\r`) characters before logging them. `strings.ReplaceAll(strings.ReplaceAll(port, "\n", ""), "\r", "")` is a simple approach to prevent log injection.

## 2025-02-27 - Missing required authentication secrets in tests
**Vulnerability:** The server `auth` package mandates that `REGISTRATION_SECRET` and `JWT_SECRET` must be set via environment variables. In several GitHub Actions workflows (like fuzz testing, deadlock detection, unit tests, and chaos testing), these variables were missing, causing `init()` functions inside `auth.go` to panic and fail the entire CI pipeline.
**Learning:** Security controls that enforce mandatory secrets at startup (e.g. failing fast if secrets are missing) will break not just the production application, but also isolated test runs or secondary tasks if those environments aren't configured with dummy secrets. It's crucial to ensure that all CI environments replicate the minimum required security configuration.
**Prevention:** Add default or dummy values for all required security secrets in GitHub Actions workflows using the `env:` block.

## 2025-02-27 - Data race in tests when modifying package-level functions
**Vulnerability:** In `agent/watchdog/main_test.go`, the test functions directly override `execCommand`, a package-level variable. Because tests can run concurrently (or wait on sleep intervals), and `defer func() { execCommand = oldExec }()` resets the variable asynchronously with other running tests, this causes a data race, triggering the `go test -race` check to fail.
**Learning:** Overriding global variables without synchronization in Go unit tests introduces data races, which makes the tests flaky and forces failure on standard `go test -race` pipelines.
**Prevention:** Avoid running data race tests in parallel without synchronization, or better yet, structure the code to allow injecting dependencies instead of mocking package globals. If overriding globals is required, ensure the tests execute synchronously and that background goroutines finish or the test avoids overriding while other goroutines might access it.
