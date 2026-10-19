## 2024-05-24 - Avoid O(N) Slice Allocations for Aggregate Statistics
**Learning:** When calculating aggregate statistics (e.g., node status counts) across the fleet, using `GetNodes()` creates unnecessary O(N) slice copies of internal maps, causing memory allocation overhead.
**Action:** Implement and use a direct `GetStats()` method on the registry that computes counts while holding the lock, skipping intermediate slice allocation.
