## 2024-07-26 - Compute aggregates directly instead of mapping to intermediate slices
**Learning:** Returning large collections to calculate aggregates causes unnecessary O(N) allocation and copying. Specifically, copying out all internal map values as slice elements (e.g. `AgentDetails`) just to count the statuses hurts performance on a large number of agents.
**Action:** Always compute aggregates directly inside the thread-safe block instead of exporting all data structures. Provide methods like `GetStats()` to calculate these aggregates in-place.
