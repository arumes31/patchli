## 2026-07-22 - Optimize HandleStats by reducing allocations
**Learning:** When calculating aggregate statistics across the fleet, avoid creating O(N) slice copies of internal maps; instead, use specialized manager methods that iterate directly over the data structure to reduce memory allocation overhead.
**Action:** Iterate directly over the data structure when gathering stats instead of copying all nodes first.
