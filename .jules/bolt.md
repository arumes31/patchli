## 2024-07-23 - Optimize Fleet Status Aggregation
**Learning:** Calculating aggregate statistics by first copying the entire map into a slice (e.g., via GetNodes) causes unnecessary O(N) memory allocation and GC pressure, particularly as the fleet grows.
**Action:** Implement specialized map-iteration methods (like GetStats) that compute aggregates directly while holding the read lock, avoiding intermediate slice copies.
