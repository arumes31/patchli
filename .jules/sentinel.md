## 2024-06-11 - Script Injection in Setup Scripts
**Vulnerability:** Command / Script Injection in setup script generation where user-provided parameters (group, timestamp, signature, baseURL) were interpolated directly into shell/PowerShell scripts using double quotes, allowing execution of arbitrary commands.
**Learning:** When generating configuration or setup scripts dynamically with user input, wrapping inputs in double quotes is unsafe as it allows shell variable expansion and command substitution.
**Prevention:** Always use single quotes for interpolation in shell/PowerShell scripts and properly escape internal single quotes (e.g., using `'\''` for bash and `''` for PowerShell) before injection.
