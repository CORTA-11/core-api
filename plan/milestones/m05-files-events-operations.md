# M05 — Files, events, and operations

| Field | Value |
| --- | --- |
| Status | `not started` |
| Outcome | Managed files, durable background work, realtime hints, telemetry, and recovery turn the reference slice into an operable core release. |
| Depends on | M04 complete; TDR-03 through TDR-05 where applicable |
| Release | Core release |

## Deliverables

### M05-D01 — File metadata and authorization

**Artifacts:** tenant file migrations/queries, `internal/file/` domain/service,
OpenAPI paths.

- [ ] Store file public ID, team owner, display name, media type, expected/actual
  size, content hash, storage key, lifecycle state, version, creator, and
  timestamps in PostgreSQL.
- [ ] Use opaque server-generated object keys containing public/random
  identifiers, never client filenames or internal team IDs.
- [ ] Authorize metadata/list/download/delete through the same tenant executor
  and permission service as tasks.
- [ ] Define `pending → uploading → available/failed/deleted` transitions and
  reconcile abandoned or orphaned records/objects.

**Acceptance:** wrong-team guessed ID, invalid transition, duplicate completion,
missing object, orphan object, delete/retry, and metadata/object disagreement
tests pass.

### M05-D02 — Bounded presigned multipart transfer

**Artifacts:** MinIO adapter, upload handlers/contracts, cleanup jobs.

- [ ] Implement initiate, sign-part, complete, abort, download-sign, and status
  operations with bounded file size, part size/count, TTL, and concurrent
  uploads per actor/team.
- [ ] Bind every operation to file public ID, tenant context, current lifecycle,
  and expected metadata; never accept arbitrary bucket/key input.
- [ ] Verify completed part manifest and object size before marking available;
  compute/verify the selected content hash in a bounded workflow.
- [ ] Keep bounded server streaming only for small/admin flows with explicit
  content length and cancellation.

**Acceptance:** oversized body/file, too many/small parts, expired URL, wrong
tenant, forged key, incomplete manifest, duplicate complete/abort, disconnect,
and MinIO timeout tests pass against real MinIO.

### M05-D03 — Durable worker and outbox delivery

**Artifacts:** `cmd/worker/`, River (or approved PostgreSQL-backed job library)
migrations/config, outbox dispatcher and handlers.

- [ ] Enqueue durable work in the same database transaction as its domain state
  or through the transactional outbox.
- [ ] Define typed payloads, uniqueness/idempotency keys, attempt/time limits,
  retry backoff, terminal failure, cancellation, and bounded concurrency.
- [ ] Implement initial jobs for outbox dispatch, file reconciliation/hash, and
  abandoned-upload cleanup.
- [ ] Expose bounded job inspection/retry through an authenticated operator
  command, not a public endpoint.

**Acceptance:** crash after claim, crash after side effect, duplicate delivery,
poison payload, timeout, cancellation, and exhausted retry tests show no duplicate
domain state.

### M05-D04 — Realtime contract and adapter

**Artifacts:** versioned event envelope, AsyncAPI contract, Centrifugo publish
and subscription-authorization adapters, integration tests.

- [ ] Publish team/task/file hints only after commit, using public identifiers,
  event version, aggregate version, and event ID.
- [ ] Authorize every subscription against current session and team membership;
  do not trust a client-provided channel name.
- [ ] Treat realtime as a hint: clients recover through authoritative paginated
  HTTP state after disconnect, loss, duplication, or reordering.
- [ ] Keep Redis/Centrifugo failure from rolling back already committed domain
  state; retry through durable work.

**Acceptance:** forged channel, revoked member, duplicate/reordered/lost event,
multi-node publish, Redis outage, and reconnect-from-cursor tests pass.

### M05-D05 — Confidential-mode seam (not implementation by default)

**Artifacts:** privacy-mode field/contract and adapter boundary only after
`TDR-03`; no custom cryptography.

- [ ] Keep the core release explicitly `managed` unless TDR-03 is approved and
  corresponding client implementation exists.
- [ ] If enabled, store only ciphertext, authenticated encryption metadata,
  wrapped data keys, and key versions; never plaintext team keys.
- [ ] Disable server-side preview/search/AI for confidential bytes unless a later
  approved protocol supplies plaintext with explicit consent.
- [ ] Document accurately that revocation cannot erase copies already decrypted
  by an authorized device.

**Acceptance:** managed-only builds make no confidential claim. An enabled mode
must pass ciphertext inspection, tamper, wrong-key/version, revocation, rotation,
and log-redaction tests before release.

### M05-D06 — Observability and bounded degradation

**Artifacts:** OpenTelemetry wiring, metrics, structured request/job fields,
dashboards/alerts as code where owned here.

- [ ] Trace HTTP → tenant transaction → outbox/job → MinIO/realtime using request
  and event IDs without recording tokens, SQL text with values, or file content.
- [ ] Emit bounded-cardinality latency/error/saturation metrics for HTTP, local
  credential verification, sessions, pgx, jobs, Redis, MinIO, and outbox lag.
- [ ] Define and test dependency deadlines, pool/queue limits, circuit/open
  behavior, and readiness effects.
- [ ] Preserve M03's Redis split: cache/realtime/telemetry failure degrades,
  while login and administrative mutations return a bounded `503` when their
  Redis rate limiter is unavailable. PostgreSQL authorization/session or required
  storage failure remains fail-closed.

**Acceptance:** telemetry redaction/cardinality tests and PostgreSQL, Redis,
MinIO, local-auth/session, realtime, and telemetry outage scenarios pass with
bounded response/job times and the M03 Redis failure classifications intact.

### M05-D07 — Backup, restore, and core release runbook

**Artifacts:** `ops/backup`, `ops/restore` or safe command equivalents,
deployment/runbook files, release Compose profile.

- [ ] Back up PostgreSQL plus MinIO objects and the configuration/key references
  required to restore them; Redis/realtime state is disposable.
- [ ] Restore into a validated empty target, apply/verify migration versions, and
  compare tenant counts plus representative object hashes.
- [ ] Document start, migrate, rollback, shutdown, failed provisioning/job,
  storage outage, credential/session incident, and restore commands with exact
  safety checks.
- [ ] Measure the first isolated restore and use it to close TDR-05; do not claim
  untested availability.

**Acceptance:** `make test-e2e-core`, `make test-failure`, and
`make test-restore` pass in a disposable environment. The core release uses
immutable build artifacts and starts with the runtime role.

## Merge order

D01 → D02; D03 may start after M04-D05; D04 depends on D03. D06 applies to every
new adapter. D07 closes the core release. D05 is optional and cannot delay the
managed-files release.

**Implementation links:** _none yet_.
