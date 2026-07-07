## 2024-07-07 - Avoid O(N) allocation for stats calculation
**Learning:** Returning large slices just to loop over them for computing counts leads to unnecessary O(N) allocations and copy overhead.
**Action:** Expose a method like `GetStats()` that computes counts directly on the internal map using a read lock.
