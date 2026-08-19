# M07 — Collaboration

| Field | Value |
| --- | --- |
| Status | `not started` |
| Outcome | Durable chat, versioned documents, notifications, realtime recovery, and managed search operate within the same authorization boundary. |
| Depends on | M06 research release complete |
| Release | Collaboration release |

## Deliverables

### M07-D01 — Durable team chat

**Artifacts:** chat migrations/queries, `internal/chat/`, HTTP and realtime
contracts.

- [ ] Store channel/message public IDs, team ownership, sender, bounded content,
  client idempotency key, version/edit/delete state, and timestamps under FORCE
  RLS.
- [ ] Implement paginated history, send, edit, and delete with explicit
  permission and retention behavior.
- [ ] Commit message, audit, and outbox event atomically; publish realtime hints
  through the M05 delivery path.
- [ ] Recover after disconnect from authoritative history using a stable cursor.

**Acceptance:** wrong channel/team, revoked sender/subscriber, duplicate send,
stale edit, deleted message, event loss/reorder, Redis outage, and reconnect tests
pass.

### M07-D02 — Versioned documents

**Artifacts:** document/version migrations, service/handlers/contracts.

- [ ] Add team-owned document identity plus immutable content versions with
  bounded managed content, hash, author, optimistic version, and lifecycle.
- [ ] Implement get/list/create/update/publish/delete using ETags and idempotency.
- [ ] Link exact document versions into research provenance where authorized.
- [ ] Use ordinary optimistic concurrency; CRDT/co-editing is outside this
  milestone.

**Acceptance:** concurrent edit returns one success and one stale response;
published-version mutation, oversized content, cross-team provenance, replay,
and RLS tests pass.

### M07-D03 — Durable notifications

**Artifacts:** notification/preferences migrations, worker handlers, APIs.

- [ ] Derive in-app notifications from durable domain events with recipient
  authorization checked at creation and again at read/delivery where needed.
- [ ] Store bounded read/dismiss/delivery state and per-user/team preferences.
- [ ] Deduplicate at-least-once event handling and avoid exposing protected
  object details after access is revoked.
- [ ] Provide bounded paginated list and unread-count endpoints.

**Acceptance:** duplicate event, revoked membership, deleted source, preference
change, retry, pagination, and unread-count concurrency tests pass.

### M07-D04 — Authorized managed search

**Artifacts:** PostgreSQL search columns/indexes/queries, search service/API,
reindex job.

- [ ] Index approved managed task, artifact, experiment, chat, and document
  fields in PostgreSQL; do not index confidential bytes.
- [ ] Apply tenant/team authorization in the query boundary before ranking and
  return public IDs plus typed source/version references.
- [ ] Bound query length, filters, page size, execution time, result excerpts,
  and reindex concurrency.
- [ ] Update search state from durable events and provide a resumable rebuild.

**Acceptance:** wrong-team term leakage, revoked source, retracted/deleted state,
confidential exclusion, expensive query bound, event replay, and rebuild tests
pass.

### M07-D05 — Collaboration conformance suite

**Artifacts:** multi-client E2E/failure/isolation tests and updated contracts.

- [ ] Exercise two clients across send/edit/reconnect, document conflict,
  notification delivery, and search indexing.
- [ ] Kill/restart worker and realtime dependencies and reconcile from durable
  state.
- [ ] Re-run tenant/role matrices for every collaboration table and channel.
- [ ] Verify log/trace/event payloads exclude forbidden content and stay within
  cardinality/size bounds.

**Acceptance:** the M07 demonstration in `verification.md` passes and M01-M06
regression suites remain green.

## Merge order

D01 and D02 can proceed independently after the M05 event contract. D03 consumes
their events. D04 follows stable domain schemas. D05 closes the release.

**Implementation links:** _none yet_.
