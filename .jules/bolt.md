## 2026-06-17 - Reuse Case-Insensitive String Operations within Loops
**Learning:** Calling `strings.ToLower()` multiple times on the same string within a loop iteration is a common, hidden source of unnecessary string allocations and CPU overhead, especially when checking for multiple substrings via `strings.Contains()`.
**Action:** Extract the lowercased string into a local variable within the loop before executing the checks.
