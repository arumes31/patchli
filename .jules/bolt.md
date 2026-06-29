## 2024-06-29 - Cache String Operations in Loops
**Learning:** Performing multiple case-insensitive operations (e.g., equality and containment) on the same string within a loop without caching `strings.ToLower()` creates redundant memory allocations, reducing performance when iterating over large datasets (like fleet nodes).
**Action:** When performing multiple case-insensitive checks on the same variable within a loop, assign the result of `strings.ToLower()` to a local variable to minimize allocations and improve efficiency.
