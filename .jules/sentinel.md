## 2025-02-27 - Script Injection in Setup Script Generation
**Vulnerability:** The `/setup` endpoint generated bash and powershell setup scripts by directly injecting user-provided input (like `group` from the URL query) into string templates using double quotes (e.g. `GROUP="%s"` or `$Group = "%s"`).
**Learning:** Bash and Powershell will parse and potentially execute contents within double quotes, allowing command injection. Injected variables must be surrounded by single quotes.
**Prevention:** When generating shell or powershell scripts dynamically, enclose dynamic variables in single quotes (`'`) instead of double quotes (`"`) and escape any single quotes in the variable itself (using `'\''` for bash and `''` for powershell) to prevent execution or expansion of malicious strings.
