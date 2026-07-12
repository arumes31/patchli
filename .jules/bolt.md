## 2024-07-12 - Avoid O(N) Slice Allocations for Map Aggregation
**Learning:** Returning a large internal map as a copied slice (e.g., `GetNodes()`) just to calculate aggregate counts causes severe O(N) memory allocation overhead as the fleet grows.
**Action:** Implement specialized manager methods (like `GetStats()`) that calculate aggregates by iterating directly over the internal map while holding a read lock, preventing unnecessary copying.
