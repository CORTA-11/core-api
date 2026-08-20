# M06 — Research operations

| Field | Value |
| --- | --- |
| Status | `not started` |
| Outcome | Research artifacts, experiments, provenance, and resource bookings form a tenant-safe, attributable workflow on the core platform. |
| Depends on | M05 core release complete |
| Release | Research release |

## Deliverables

### M06-D01 — Artifact registry and immutable versions

**Artifacts:** tenant migrations/queries, `internal/artifact/`, OpenAPI paths.

- [ ] Add artifact identity, type, owner team, lifecycle, metadata, and immutable
  version rows linked to managed file IDs or bounded structured content.
- [ ] Store content hash, version number, creator, creation time, and publication
  state with uniqueness/foreign-key/check constraints and FORCE RLS.
- [ ] Permit draft updates by creating versions; publishing fixes an exact
  version and retraction preserves history.
- [ ] Implement bounded list/get/create-version/publish/retract endpoints with
  idempotency, ETags, audit, and outbox events.

**Acceptance:** mutation of a published version, cross-team link, duplicate
version/hash ambiguity, stale publish, replay, retract, and RLS tests pass.

### M06-D02 — Experiment registry and runs

**Artifacts:** experiment/run migrations, domain/service/handlers, OpenAPI.

- [ ] Store experiment identity, protocol/config version, lifecycle, owning team,
  creator, timestamps, and optimistic version.
- [ ] Store each run with explicit parameters, status transitions, start/end,
  input/output references, and bounded failure metadata.
- [ ] Validate transitions transactionally and execute asynchronous run-related
  processing through typed durable jobs.
- [ ] Keep external execution adapters behind explicit timeout/idempotency and
  result-validation boundaries.

**Acceptance:** invalid/stale transition, duplicate callback/job, missing input,
wrong-team reference, cancellation, worker crash, and oversized parameters fail
without partial state.

### M06-D03 — Provenance graph

**Artifacts:** provenance edge migrations/queries/services and read endpoints.

- [ ] Link exact artifact versions, experiment versions/runs, file versions,
  inputs, and outputs using constrained typed edges.
- [ ] Prevent cross-team/organization edges, self-links where invalid, duplicate
  edges, and cycles for relationships declared acyclic.
- [ ] Provide bounded upstream/downstream traversal with depth/node limits and
  deterministic pagination.
- [ ] Include actor/request/job attribution in audit records without copying
  unbounded scientific content.

**Acceptance:** wrong-version, cycle, cross-team, missing node, depth/node bound,
and concurrent duplicate edge tests pass with real PostgreSQL.

### M06-D04 — Resource catalog and conflict-safe booking

**Artifacts:** resource/booking migrations, domain/service/handlers, OpenAPI.

- [ ] Add resource identity, team visibility, availability state, and bounded
  metadata.
- [ ] Store bookings using PostgreSQL range types and an exclusion constraint so
  overlapping active bookings for one resource cannot both commit.
- [ ] Implement create/update/cancel/list with time-zone-explicit timestamps,
  idempotency, ETags, audit, and notification outbox events.
- [ ] Define transaction isolation/retry behavior for concurrent booking changes.

**Acceptance:** two truly concurrent overlapping creates yield one commit;
adjacent/non-overlapping slots succeed; stale update, cancelled booking, invalid
range, wrong team, and retry tests pass.

### M06-D05 — Research notifications and export

**Artifacts:** job handlers, notification event types, bounded export format.

- [ ] Generate durable events for publish/retract, run completion/failure, and
  booking create/change/cancel.
- [ ] Deduplicate delivery by event/subscriber/channel and retain bounded status
  without making delivery block domain commits.
- [ ] Export an artifact/run/provenance bundle with exact public IDs, versions,
  hashes, and timestamps; authorize every included object.
- [ ] Record export creation and hash as an audited durable job.

**Acceptance:** duplicate delivery, revoked recipient, retry, partial export,
cross-team graph, and export reproduction/hash tests pass.

### M06-D06 — Research workflow conformance

**Artifacts:** end-to-end, isolation, concurrency, and query-plan tests.

- [ ] Exercise draft artifact → version → experiment run → output → publish →
  provenance traversal and resource booking through HTTP and the worker.
- [ ] Re-run tenant isolation and role matrix coverage for every new table/path.
- [ ] Verify access-path indexes and bounded traversal/list behavior using
  representative fixtures.
- [ ] Add the new contract/events to OpenAPI/AsyncAPI drift checks.

**Acceptance:** the M06 demonstration in `verification.md` passes and the core
M01-M05 suites remain green.

## Merge order

D01 → D02 → D03; D04 can proceed after the M04 transaction pattern is stable.
D05 consumes D01-D04 events; D06 closes the research release.

**Implementation links:** _none yet_.
