## 2024-06-04 - [String Optimization]
**Learning:** The codebase previously contained a pattern where `strings.ToLower()` was called multiple times on the same field inside a loop (`HandleStats` function in `server/internal/api/handlers.go`).
**Action:** Always assign the result of string manipulation operations like `strings.ToLower()` to a local variable if it needs to be used for multiple conditions within a loop. This avoids redundant allocations and improves performance significantly on large lists.
