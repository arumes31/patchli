## 2024-07-21 - [Avoid O(N) memory allocation when aggregating stats]
**Learning:** [When calculating aggregate statistics across the fleet, avoid creating O(N) slice copies of internal maps.]
**Action:** [Use specialized manager methods (like GetStats) that iterate directly over the data structure to reduce memory allocation overhead.]
