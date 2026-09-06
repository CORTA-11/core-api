---
status: accepted
---

# Core API owns durable collaborative documents

Collaborative Documents need both low-latency live synchronization and durable recovery. The core API and tenant Postgres remain authoritative for persisted document state, while `socket-server` owns live synchronization and presence; this preserves the existing ownership boundary and prevents ephemeral socket or Redis state from becoming the only copy of team content.
