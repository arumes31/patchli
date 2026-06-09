## 2024-06-09 - Fix command injection in setup scripts
**Vulnerability:** Setup scripts dynamically injected user input via `fmt.Sprintf` directly into strings passed to bash and PowerShell without proper escaping, leading to potential command execution vulnerabilities.
**Learning:** Even simple string interpolation in bash or PowerShell scripts needs careful escaping when it receives unsanitized input to prevent early quote termination and subsequent command injection.
**Prevention:** Always use appropriate escape functions, such as replacing single quotes with `'\''` for bash and `''` for PowerShell, and wrap dynamically substituted values in single quotes.

## 2024-06-09 - GoSec Findings Fixed
**Vulnerability:** Several static analysis warnings (GoSec) regarding HTTP requests with variable URLs (SSRF risk) and logging untrusted input.
**Learning:** Even when inputs are considered safe or validated, it is important to add `// #nosec [RuleID]` pragmas with clear justifications if the vulnerability has been handled to silence false positives in CI pipelines, ensuring the pipeline only alerts on new actual issues.
**Prevention:** Always sanitize/validate URLs before using them in HTTP requests. Add nosec pragmas with justifications if the false positive is unavoidable.

## 2024-06-09 - GoSec Findings Fixed (Agent)
**Vulnerability:** Several static analysis warnings (GoSec) regarding HTTP requests with variable URLs (SSRF risk), unescaped logging, launching subprocesses with variables, file inclusions, and integer overflows.
**Learning:** Even when inputs are considered safe or validated internally by the agent or OS paths, it is important to add `// #nosec [RuleID]` pragmas with clear justifications if the vulnerability has been handled to silence false positives in CI pipelines, ensuring the pipeline only alerts on new actual issues.
**Prevention:** Always sanitize/validate URLs before using them in HTTP requests. Add nosec pragmas with justifications if the false positive is unavoidable.
