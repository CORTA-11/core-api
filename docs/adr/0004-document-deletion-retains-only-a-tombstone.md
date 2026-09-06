---
status: superseded by ADR-0005
---

# Document deletion retains only a tombstone

Deleting a Document immediately erases its canonical Yjs state and materialized projections but retains a content-free tombstone. The tombstone rejects delayed collaboration checkpoints so a stale Document Room cannot recreate deleted content, while retaining no recoverable document revision.
