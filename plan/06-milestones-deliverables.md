# Technical milestones and deliverables

This file is the delivery dashboard. Detailed scope and acceptance checks live
in the linked milestone files.

## Status

| Milestone | Status | Merge-sized deliverables | Release result |
| --- | --- | ---: | --- |
| [M01 — Reproducible baseline](milestones/m01-reproducible-baseline.md) | `complete` | 5 | Reliable local/CI development loop |
| [M02 — Trusted tenant boundary](milestones/m02-trusted-tenant-boundary.md) | `complete` | 5 | Database-enforced organization/team isolation |
| [M03 — Identity, authorization, and API](milestones/m03-identity-authorization-api.md) | `not started` | 6 | Authenticated, default-deny `/api/v1` contract |
| [M04 — Team and task reference slice](milestones/m04-team-task-slice.md) | `not started` | 6 | Complete concurrent Kanban backend slice |
| [M05 — Files, events, and operations](milestones/m05-files-events-operations.md) | `not started` | 7 | Core backend release |
| [M06 — Research operations](milestones/m06-research-operations.md) | `not started` | 6 | Research registry and booking release |
| [M07 — Collaboration](milestones/m07-collaboration.md) | `not started` | 5 | Chat, documents, notifications, and managed search |
| [M08 — Authorization-aware AI](milestones/m08-authorization-aware-ai.md) | `not started` | 5 | Optional, controlled AI capabilities |

`M01` completed in [PR #21](https://github.com/CORTA-11/core-api/pull/21) at
merge commit `f2ee418`. `M02` completed in
[PR #27](https://github.com/CORTA-11/core-api/pull/27) at merge commit `56d0a6d`.

## Dependency and release cuts

| Release cut | Required milestones | Shippable behavior |
| --- | --- | --- |
| Engineering baseline | M01 | Contributors and CI reproduce the same build and test result. |
| Security foundation | M01-M03 | Authenticated requests execute only inside a trusted tenant/team boundary. |
| Core release | M01-M05 | Organizations, teams, concurrent tasks, managed files, durable events, telemetry, and recovery operate end to end. |
| Research release | M01-M06 | Artifacts, experiments, provenance, and conflict-safe resource booking are added. |
| Collaboration release | M01-M07 | Durable chat, documents, notifications, realtime recovery, and authorized managed search are added. |
| AI release | M01-M08 | Approved AI capabilities use authorization-filtered context and typed human-approved actions. |

M06-M08 are additive release increments. The core release does not wait for AI.

## Critical path

1. Finish M01-D01 through M01-D04; M01-D05 can expand alongside M02.
2. Land the sqlc split before adding tenant migrations that depend on separate
   public and tenant repositories.
3. Land tenant provisioning and the tenant executor before routing any request
   into a tenant schema.
4. Prove RLS and pool cleanup with real PostgreSQL before replacing public
   routes.
5. Land OIDC sessions and authorization before exposing `/api/v1` team data.
6. Use teams/tasks as the reference slice; later domains copy its transaction,
   authorization, contract, audit, idempotency, and test pattern.
7. Ship managed files and durable events before research/collaboration breadth.

## Milestone completion rule

A milestone moves to `complete` when all of its deliverable checkboxes are done,
its migrations work from both an empty database and the previous milestone, its
contract matches the running handler, and its demonstration in
[`verification.md`](verification.md) passes in CI or a documented disposable
environment. A status update must link the implementing commits or pull requests
inside the milestone file.

## De-scoped from the critical path

- rewriting the four long-form guidance documents;
- assigning committees or creating evidence ledgers;
- flag-day conversion to a speculative package tree;
- frontend or Python toolchains in this repository;
- confidential file claims before `TDR-03` is closed;
- Kubernetes, multi-region, service mesh, Kafka, CRDT, and custom OAuth or
  cryptographic primitives.
