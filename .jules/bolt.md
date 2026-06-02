## 2025-06-02 - time.Parse in PruneStaleAgents Lock Contentions
**Learning:** `time.Parse` inside a fleet-wide loop while holding a write lock (`am.mu.Lock()`) causes O(N) blocking on all agent operations (registration, unregistration, etc.). This string parsing overhead becomes a severe bottleneck as the agent fleet grows.
**Action:** Always store timestamps as `time.Time` instead of `string` if they are frequently compared or queried in loops, especially inside mutex locks.
