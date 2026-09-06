---
status: superseded by ADR-0005
---

# Persist bounded Yjs checkpoints

The collaboration service coalesces changes after 250 milliseconds of inactivity but waits no more than one second before sending a complete checkpoint to the core API. Each atomic commit uses an expected database revision to reject stale writers and returns a monotonically increasing revision plus the committed Yjs state vector; only that acknowledgement permits the UI to report the Document as saved.
