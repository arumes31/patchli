## 2025-02-27 - Security fixes and false positives
**Vulnerability:** Found multiple vulnerabilities and Gosec warnings including:
1. Agent Gosec failures due to `G204: Subprocess launched with a potential tainted input or cmd arguments`.
2. Gosec failures due to `G304: Potential file inclusion via variable`.
3. Gosec failure `G115: integer overflow conversion int64 -> uint64` in `CheckDiskSpace`.
4. Gosec failure `G302: Expect file permissions to be 0600 or less` in agent file update functions.
5. Gosec `G704: SSRF via taint analysis` in agent polling endpoint.
6. Test failure in `state_test.go` due to using invalid characters `Z:\invalid\path\?:|/state.json` on Linux OS.
7. Test failure in `apt_test.go` due to hardcoded usage of `cmd.exe` on Linux runners to simulate process locking.
**Learning:** Security analysis tools like Gosec often raise false positives or flag intended behavior as vulnerabilities (e.g., an agent executing scripts provided by its command server). When the functionality is by design, you must carefully suppress these warnings with `#nosec` comments, while still enforcing tight controls around where those inputs come from. Cross-platform tests should also avoid hardcoding OS specific features like `cmd.exe` or `Z:` paths to trigger errors.
**Prevention:**
- For necessary OS executions, add explicit `#nosec G204` tags accompanied by a comment justifying why it's secure (e.g., verifying that the input is from a trusted control plane).
- Avoid running data race tests in parallel without synchronization or use `sync.RWMutex` if overriding package globals is required in unit tests.
- Provide `env:` overrides for tests in GitHub actions to pass if code expects `os.Getenv` requirements to be set.
