## 2026-07-24 - Optimize Aggregation Calls
**Learning:** When calculating aggregate statistics (e.g., node status counts) across the fleet, avoid creating O(N) slice copies of internal maps; instead, use specialized manager methods (like GetStats) that iterate directly over the data structure to reduce memory allocation overhead.
**Action:** Add GetStats method in Manager instead of using GetNodes when aggregation is the only goal.
