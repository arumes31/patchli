## 2025-06-25 - Caching string transformations in loops
**Learning:** Performing `strings.ToLower()` multiple times inside loops (e.g. `HandleStats`) introduces unnecessary string allocations and cpu cycles, as `strings.ToLower` returns a new string each time.
**Action:** When evaluating case-insensitive conditions multiple times on the same string in a loop, extract `strings.ToLower()` to a local variable to cache the result, minimizing allocations and improving loop performance.
