## 2024-05-18 - Cached ToLower result in loop
**Learning:** Found an anti-pattern of calling `strings.ToLower()` multiple times on the same field inside a loop iterating over fleet nodes. This operation causes unnecessary heap allocations.
**Action:** When performing multiple case-insensitive operations on the same string in a loop, assign the `strings.ToLower()` result to a local variable to halve string allocations.
