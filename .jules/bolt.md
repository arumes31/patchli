## 2025-02-18 - Avoiding O(N) Allocations in Aggregations
**Learning:** Returning a full copy of a collection (like a map's values as a slice) just to compute aggregate statistics can lead to unnecessary O(N) memory allocations, negatively impacting performance (time and GC).
**Action:** Provide specialized aggregate methods (e.g. `GetStats()`) directly on the data structure manager to iterate and compute counts internally without allocating new collections.
