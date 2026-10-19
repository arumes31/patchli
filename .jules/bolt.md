## 2024-05-18 - Avoid GetNodes for Fleet Stats
**Learning:** `AgentManager.GetNodes()` (`fleet.Registry.GetNodes()`) iterates over all details and appends to a slice, returning a new slice. Calling this merely to compute fleet statistics creates unnecessary O(N) memory allocations.
**Action:** Implement a `GetStats()` method on `AgentManager` that locks, iterates over internal details to compute the statistics (total, online, reboot), and directly returns the aggregated counts to avoid memory allocation overhead for large fleets.
