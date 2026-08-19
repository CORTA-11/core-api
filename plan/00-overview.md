# Core API technical delivery plan

## Goal

Turn the current API prototype into a releasable, tenant-safe backend through
small technical milestones. This plan covers code owned by `core-api`: Go API
and worker processes, PostgreSQL migrations and queries, Redis/MinIO adapters,
API contracts, tests, and local/CI runtime configuration.

The long-form product, style, contribution, and governance documents remain
constraints and reference material. Updating those documents is not a delivery
milestone. A decision blocks implementation only when the choice changes an
interface, data model, or security boundary listed in
[`05-open-questions.md`](05-open-questions.md).

## Baseline (2026-08-19)

The repository already has Go/chi handlers, pgx/sqlc persistence, public and
per-organization migrations, basic password/JWT code, Redis team caching,
streaming MinIO access, colocated tests, and a GitHub Actions workflow.
`GOCACHE=/tmp/core-api-go-cache go test ./...` passes.

The prototype is not safe to expose as a multi-tenant service yet:

- `X-Org-ID` selects a computed schema without proving user membership;
- raw schema strings cross handler/service boundaries and are interpolated into
  `SET LOCAL search_path`;
- tenant tables have no team RLS and `tasks.team_id` is nullable;
- most routes are unauthenticated and `/debug/pprof` is public;
- APIs are unversioned, unbounded, and return ad hoc text errors;
- organization creation can commit before tenant migrations succeed;
- file authorization is inferred from an object-key prefix and uploads are
  unbounded; and
- the process lacks validated configuration, graceful shutdown, readiness,
  durable jobs, audit records, and real infrastructure tests.

## Delivery sequence

```text
M01 reproducible baseline
  -> M02 trusted tenant boundary
    -> M03 identity, authorization, and API contract
      -> M04 teams and tasks reference slice
        -> M05 files, events, and operations       [core release]
          -> M06 research operations               [research release]
            -> M07 collaboration                   [collaboration release]
              -> M08 authorization-aware AI        [optional AI release]
```

Each arrow is a dependency for production use, not a ban on design spikes. A
milestone is complete only when its code, migrations, contract, negative tests,
and operational behavior have landed together. The milestone register and
current status are in
[`06-milestones-deliverables.md`](06-milestones-deliverables.md).

## What makes a deliverable concrete

Every deliverable in this plan specifies:

1. the repository artifact to add or change;
2. the externally observable behavior;
3. the automated test or runnable demonstration that proves it;
4. its predecessor deliverables; and
5. a merge-sized implementation order.

“Write a policy,” “define a process,” and “collect evidence” are not standalone
deliverables. Documentation is included only when it describes a command,
contract, migration, runbook, or behavior implemented in the same change.

## First implementation queue

These are the first mergeable changes; later work should not jump ahead of the
tenant and authorization boundary.

| Order | Work item | Result |
| ---: | --- | --- |
| 1 | M01-D01 | `make check`, `make test-unit`, and `make test-integration` become explicit, non-secret command paths. |
| 2 | M01-D02 | CI calls the same commands and checks generated-code and migration drift. |
| 3 | M01-D03 | Disposable PostgreSQL/Redis/MinIO integration fixtures run locally and in CI. |
| 4 | M01-D04 | Typed config, startup validation, health endpoints, signal shutdown, and opt-in pprof land. |
| 5 | M02-D01 | sqlc public/tenant query packages are separated and `SELECT *` is removed from touched queries. |
| 6 | M02-D02 | Organization provisioning and tenant migration status become resumable and inspectable. |
| 7 | M02-D03 | One tenant executor owns transaction setup, trusted `search_path`, user/team settings, commit, and rollback. |
| 8 | M02-D04 | Team membership and `FORCE ROW LEVEL SECURITY` protect team-owned rows. |
| 9 | M02-D05 | Real PostgreSQL tests prove forged-tenant, broken-query, and pool-reuse isolation. |
| 10 | M03-D01 | OIDC-backed sessions replace public use of the prototype bearer-JWT routes. |

## Scope control

- The current horizontal packages may be changed incrementally; a flag-day
  package rewrite is not a prerequisite.
- The first releasable product slice is organization/team/task/file operation,
  not the full product catalog.
- Redis is an optimization and fan-out mechanism, never the authority for
  committed domain state.
- Managed file storage ships before confidential storage. Confidential mode
  requires the key-custody decision in `TDR-03` and client work outside this
  repository.
- Collaboration starts after the core release. AI starts after collaboration
  and is independently optional.
- Web UI, standalone AI service, Centrifugo deployment, and production platform
  manifests need an owning repository. This repository delivers their contracts
  and adapters, not fictional implementations.

## Planning files

- [`06-milestones-deliverables.md`](06-milestones-deliverables.md) — status,
  dependencies, and release cuts.
- [`milestones/`](milestones/) — buildable deliverables and acceptance tests for
  M01-M08.
- [`verification.md`](verification.md) — canonical test lanes and milestone
  demonstrations.
- [`05-open-questions.md`](05-open-questions.md) — the small set of technical
  choices that can actually block code.
