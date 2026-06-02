
## 2026-06-02: Optimized Heartbeat Marshaling in Patchli Agent
- **Issue**: Redundant JSON marshaling of HeartbeatPayload in every 30s poll/heartbeat interval, even when data hasn't changed.
- **Fix**: Implemented caching of the marshaled JSON payload. Re-marshal only occurs if `hostname` or `rebootNeeded` status changes.
- **Impact**: Reduced CPU cycles and memory allocations in the long-running agent polling loop.
