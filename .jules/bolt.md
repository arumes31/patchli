## 2024-05-18 - Optimize HandleStats loop
**Learning:** Calling `strings.ToLower()` multiple times inside a loop for the same string creates unnecessary allocations and CPU overhead.
**Action:** Extract repeated string operations into a local variable inside the loop.
