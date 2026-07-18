## 2025-07-18 - Optimize aggregate stats calculation over fleet state
**Learning:** For fetching aggregate counts over fleet state map, allocating and creating an O(N) slice copy is wasteful. Avoid creating slices just to iterate over values when simple integer tracking can be updated directly from the map source.
**Action:** When calculating statistics or finding matching elements in an internal map, use specialized manager methods (like GetStats) that iterate directly over the data structure, preventing unnecessary O(N) allocation and garbage collector overhead.
