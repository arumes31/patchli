## 2024-06-09 - Minimize String Allocations in Loops
**Learning:** Found a performance bottleneck where `strings.ToLower()` was being called multiple times on the same string within a loop.
**Action:** Always assign the result of `strings.ToLower()` or similar string operations to a local variable to prevent redundant allocations and processing overhead when used multiple times in the same context.
