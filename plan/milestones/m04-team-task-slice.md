# M04 — Team and task reference slice

| Field | Value |
| --- | --- |
| Status | `not started` |
| Outcome | Teams and Kanban tasks become the first complete vertical slice, including constraints, authorization, concurrency, idempotency, audit, events, tests, and contract. |
| Depends on | M03 complete |
| Release | Core release |

## Public contract target

```text
GET    /api/v1/orgs/{org_id}/teams
POST   /api/v1/orgs/{org_id}/teams
GET    /api/v1/orgs/{org_id}/teams/{team_id}/tasks
POST   /api/v1/orgs/{org_id}/teams/{team_id}/tasks
GET    /api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}
PATCH  /api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}
POST   /api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}/move
DELETE /api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}
```

## Deliverables

### M04-D01 — Team and task schema

**Artifacts:** tenant migration/query updates and regenerated tenant sqlc code.

- [ ] Give tasks a public UUID, bounded title/description, closed status, sortable
  position, version, creator/updater, timestamps, and delete semantics.
- [ ] Add database checks, unique constraints, foreign keys, access-path indexes,
  and non-null team ownership; extend FORCE RLS to every new table.
- [ ] Choose a position representation that supports transactional insert/move
  and deterministic tie-breaking without rewriting an unbounded board.
- [ ] Backfill current rows without exposing internal IDs.

**Acceptance:** empty and upgrade migrations pass; constraint tests reject bad
status, missing team, duplicate public ID, invalid position, and cross-team
relationships.

### M04-D02 — Team API

**Artifacts:** team domain/service/handler/query changes and OpenAPI paths.

- [ ] Reuse M03's signed, route/scope-bound cursor codec for a bounded,
  keyset-paginated list; do not introduce a second cursor format. Add idempotent
  create.
- [ ] Generate collision-safe slugs and return public identifiers only.
- [ ] Apply organization/team permission checks and transactional audit/outbox
  writes for creation and membership-sensitive changes.
- [ ] Treat Redis team lookup as an optional cache; cache outage falls back to
  PostgreSQL and invalidation follows committed changes.

**Acceptance:** duplicate name/slug, replay, wrong organization, removed member,
Redis outage/stale entry, pagination boundary, and concurrent create tests pass.

### M04-D03 — Task read/create API

**Artifacts:** task domain/service/handler/query changes and OpenAPI paths.

- [ ] Implement task list/get/create using only the tenant executor and M03's
  bounded signed keyset pagination contract.
- [ ] Accept an idempotency key for create; persist request fingerprint and
  response in the same transaction as the task.
- [ ] Return `ETag` from the task version and stable problem responses.
- [ ] Write audit and outbox records atomically with task creation.

**Acceptance:** duplicate replay returns the original response; key reuse with a
different payload returns conflict; wrong tenant/team and unsafe-query RLS tests
pass.

### M04-D04 — Optimistic update, move, and delete

**Artifacts:** task mutation queries/services/handlers and concurrency tests.

- [ ] Require `If-Match` for update/move/delete and update only the expected
  version.
- [ ] Move tasks transactionally within one team, reject cross-team moves, and
  keep ordering unique/deterministic under concurrency.
- [ ] Return `412 Precondition Failed` for stale state and the new representation
  or ETag after success.
- [ ] Apply explicit soft-delete/restore or hard-delete semantics consistently to
  audit, events, and list/get behavior.

**Acceptance:** two-client edit, competing move, duplicate move, stale delete,
cross-team move, and rollback-after-audit/outbox-failure tests pass with the race
detector where applicable.

### M04-D05 — Transactional audit and outbox

**Artifacts:** tenant audit/outbox migrations, repositories, domain event types.

- [ ] Store actor, action, target public ID, request ID, timestamp, and minimal
  before/after metadata without secrets or unbounded content.
- [ ] Store versioned outbox events in the mutation transaction with uniqueness
  sufficient for at-least-once publication.
- [ ] Make audit append-only to the runtime role and keep logs separate from the
  durable audit trail.
- [ ] Provide bounded inspection queries for support/testing.

**Acceptance:** forced rollback leaves no task/audit/outbox partial state; replay
does not duplicate the domain effect; runtime attempts to update audit rows fail.

### M04-D06 — Reference-slice conformance suite

**Artifacts:** contract, integration, isolation, concurrency, and failure tests.

- [ ] Exercise every team/task permission and problem response through HTTP.
- [ ] Run task SQL under real FORCE RLS and the runtime role.
- [ ] Verify queries are bounded and expected indexes are selected for reference
  list/move access patterns.
- [ ] Add a reusable slice checklist/template based on implemented code, not a
  standalone policy document.

**Acceptance:** the M04 demonstration in `verification.md` passes; all new paths
are represented in OpenAPI and the generated/contract drift check is clean.

## Merge order

D01 → D02 → D03 → D04. D05 may begin with D02 but must land before any mutation
is considered complete. D06 closes the slice.

**Implementation links:** _none yet_.
