## 2024-05-19 - Repeated String Allocations in Handlers
**Learning:** Found a pattern in `HandleStats` where `strings.ToLower` was called multiple times on the same string within a tight loop during node processing. This creates redundant memory allocations and unnecessary CPU overhead on every request.
**Action:** Always assign the result of string manipulation functions (like `ToLower`) to a local variable before running multiple case-insensitive comparisons or checks in a loop.
