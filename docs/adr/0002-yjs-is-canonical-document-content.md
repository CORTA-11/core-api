---
status: accepted
---

# Yjs is canonical document content

A Document's title and rich-text body are stored together as one complete encoded Yjs state. The Hocuspocus collaboration service loads and stores that state only through private authenticated core API endpoints, derives relational title and HTML projections, and sends them with persistence writes; it never receives tenant-database credentials. The core API atomically overwrites the latest state and projections with its own timestamp and the authenticated Editor's identity. No application-level update log or prior revision is retained, and projections are read-only fallback content rather than a competing writable representation.
