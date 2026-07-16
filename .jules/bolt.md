## 2024-07-16 - Avoid O(N) slice allocations for fleet aggregate statistics
**Learning:** Calculating aggregate statistics (like counting online nodes) by first fetching all nodes into a slice using `GetNodes()` creates unnecessary O(N) memory allocations, especially as the fleet grows.
**Action:** Add specialized methods (like `GetStats()`) to the manager that iterate directly over the internal map structure with a read lock to compute counts, avoiding slice creation and reducing memory allocation overhead.
