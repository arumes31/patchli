## 2024-07-10 - Initial Setup
**Learning:** Initializing Bolt journal.
**Action:** Use this to store codebase-specific performance learnings.

## 2024-07-10 - O(N) memory allocations for aggregate stats
**Learning:** Found that generating aggregate statistics by copying maps into slice arrays (e.g. `GetNodes`) introduces unnecessary O(N) memory allocations proportional to the size of the fleet.
**Action:** Always prefer methods that compute statistics by iterating directly over underlying map structures, rather than generating intermediate slice copies.

## 2024-07-10 - Avoiding string allocations in O(N) loops
**Learning:** Found that generating `strings.ToLower` in an O(N) loop introduces string memory allocation proportional to the number of nodes.
**Action:** Instead, use string matching manually on commonly expected cases to avoid any new string allocations per loop iteration.
