## 2025-07-15 - Optimize aggregate statistics calculation
**Learning:** Returning large collections (like `GetNodes()`) just to aggregate their properties causes unnecessary O(N) memory allocations and increases GC pressure.
**Action:** Implement specialized manager methods (e.g., `GetStats`) that iterate directly over internal state with a read lock, preventing full slice copies when only aggregate counts are needed.
