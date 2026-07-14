## 2024-07-14 - Optimize HandleStats implementation
**Learning:** Calculating aggregate statistics across the fleet using `GetNodes()` creates an O(N) slice copy of internal maps, resulting in high memory allocation overhead for large fleets. Memory indicates using specialized manager methods that iterate directly over the data structure to reduce allocation overhead.
**Action:** Add a `GetStats()` method directly to the `AgentManager` that iterates over the internal `details` map while holding a read lock, calculating the stats without copying the data into a new slice.
