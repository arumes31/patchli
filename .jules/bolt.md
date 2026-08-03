## 2024-05-15 - [Avoid O(N) allocations for stats retrieval]
**Learning:** Directly iterating over an internal map with a custom getter function is more memory-efficient than returning a full slice copy and then iterating over it, especially when only aggregation metrics are needed.
**Action:** Identify where `GetNodes()` or similar slice-returning methods are used solely for counting/aggregation, and implement specialized stat-getter methods directly on the manager struct.
