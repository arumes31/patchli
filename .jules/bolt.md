## 2026-07-28 - Optimize fleet GetStats allocation
**Learning:** Calculating aggregate statistics using GetNodes() causes unnecessary O(N) memory allocation by duplicating internal maps into slices.
**Action:** Use GetStats() to iterate maps directly within a read lock.
