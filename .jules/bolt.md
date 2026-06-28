## 2024-05-18 - Caching ToLower in Loops
**Learning:** When performing multiple case-insensitive operations (e.g., equality and containment) on the same string within a loop, calling `strings.ToLower()` multiple times leads to redundant allocations and unnecessary CPU overhead.
**Action:** Assign `strings.ToLower()` to a local variable once per loop iteration to minimize allocations and improve performance.
