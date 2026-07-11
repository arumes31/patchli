## 2024-05-18 - Optimize Fleet Stats Calculation
**Learning:** Iterating directly over an internal map to calculate summary statistics avoids the O(N) memory overhead of creating a slice copy (like `GetNodes()`) solely for enumeration.
**Action:** Always prefer direct iteration inside a read lock over allocating intermediate slices when only aggregate counts or simple transformations are needed.
