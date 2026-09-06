---
status: accepted
---

# Prefer standard Yjs persistence for the first release

The first collaborative-editor release implements real tenant-scoped Document CRUD and stores encoded Yjs state through small private core API load/store endpoints used by Hocuspocus. It otherwise uses Hocuspocus's standard document lifecycle and normal Yjs reconnect-and-merge behavior. Custom durable acknowledgements, revision fencing, content-free deletion tombstones, application-level recovery protocols, and special backup exclusions are deferred so the first release remains a small working vertical slice.
