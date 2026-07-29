## 2026-07-29 - [Avoid O(N) Slice Allocations for Aggregate Statistics]
**Learning:** When calculating aggregate statistics across the fleet (e.g., node status counts), calling methods like `GetNodes()` that return a slice copy of an internal map causes unnecessary O(N) memory allocation overhead and string copies.
**Action:** Use a dedicated method (e.g. `fleet.Registry.GetStats()`) that iterates directly over the internal map under a read lock and performs simple matching logic to return counts directly, skipping the slice construction.
