## 2026-07-19 - [Avoid O(N) slice allocation for aggregate statistics]
**Learning:** Iterating directly over an internal map structure via a specialized method is more efficient than building an intermediate O(N) slice (e.g. `GetNodes()`) just to calculate aggregate counts like total, online, and reboot needed statuses. This avoids unnecessary memory allocations and copying overhead.
**Action:** When calculating aggregate statistics across the fleet, use specialized manager methods that iterate directly over the data structure rather than creating large slice copies.
