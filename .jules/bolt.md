## 2024-05-14 - String Allocation in Loop
**Learning:** Found multiple `strings.ToLower()` calls on the same string field inside a loop iterating over potentially large `nodes` slice in `server/internal/api/handlers.go`. This creates unnecessary allocations and string manipulation on every iteration.
**Action:** When performing multiple case-insensitive operations on the same string within a loop, assign `strings.ToLower()` to a local variable to minimize allocations and improve CPU efficiency.
