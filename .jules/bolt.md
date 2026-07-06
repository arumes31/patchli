## 2024-07-06 - Optimize HandleStats Memory Allocation
**Learning:** Generating aggregate statistics by first copying map contents into a new slice creates unnecessary O(N) memory allocations, negatively impacting performance at scale.
**Action:** Implement specialized map iterator methods (like GetStats) that aggregate data directly while holding a read lock, rather than returning complete data structure copies.
