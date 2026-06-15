## 2025-06-15 - Redundant string lowercasing in tight loops
**Learning:** In Go, performing `strings.ToLower()` multiple times on the same struct field within a loop creates unnecessary string allocations and CPU overhead, especially when checking against multiple string constants (e.g., "online", "reboot").
**Action:** Always assign the result of `strings.ToLower()` to a local variable before performing multiple checks (like `strings.Contains`) inside loops that iterate over slices or maps.
