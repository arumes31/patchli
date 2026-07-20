## 2024-07-20 - Aggregate Statistics Memory Overhead
**Learning:** Calculating aggregate statistics across the fleet by fetching all nodes via GetNodes() creates an O(N) slice copy of internal maps, which introduces significant memory allocation overhead for large fleets.
**Action:** Use specialized manager methods (like GetStats()) that iterate directly over the data structure while holding the read lock to eliminate unnecessary slice allocations.
