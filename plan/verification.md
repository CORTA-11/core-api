# Technical verification matrix

Tests are delivered with the behavior they prove. Reports and screenshots do
not substitute for an executable check.

## Canonical lanes to implement in M01

| Command | Environment | Required proof |
| --- | --- | --- |
| `make check` | Go toolchain only; no `.env` | format/tidy drift, build, vet/lint, sqlc drift, migration naming, and static security checks; leaves the worktree unchanged |
| `make test-unit` | Go toolchain only | handler, service, domain, middleware, config, and adapter unit tests |
| `make test-race` | Go toolchain only | `go test -race ./...` for concurrency-sensitive packages |
| `make test-integration` | disposable test Compose | real PostgreSQL, Redis, and MinIO behavior including migrations and adapter degradation |
| `make test-isolation` | disposable PostgreSQL; runtime and migration roles | organization schema isolation, team RLS, missing context, unsafe query, and pool reuse |
| `make test-contract` | API process plus disposable dependencies | OpenAPI conformance, problem responses, auth/CSRF, pagination, ETag, and idempotency behavior |
| `make test-e2e-core` | API, worker, PostgreSQL, Redis, MinIO, identity provider, realtime adapter | organization → team → task → file flow, async event delivery, and reconnect/recovery |
| `make test-failure` | disposable full stack | worker crash, duplicate job, Redis/MinIO/identity outage, shutdown, and retry bounds |
| `make test-restore` | isolated backup target only | destroy and restore durable PostgreSQL/MinIO state with hash and row-count comparison |

## Milestone demonstrations

| Milestone | Demonstration that closes the milestone |
| --- | --- |
| M01 | On a fresh checkout without `.env`, `make check` and `make test-unit` pass and do not modify tracked files; integration dependencies start and are torn down by one documented command. |
| M02 | Two organizations and two teams are alternated over a small pgx pool. Cross-organization access, a deliberately missing team predicate, missing context, and forged schema-like input all fail while same-team operations succeed. |
| M03 | A browser-style OIDC login creates a server session; missing/expired session, bad CSRF, revoked membership, and insufficient permission return the documented problem types without tenant data leakage. |
| M04 | Two clients update and move the same task. One succeeds, one receives `412`; replaying a create request returns the original result; audit/outbox rows commit exactly once. |
| M05 | A bounded multipart upload completes through opaque object keys; a worker crash safely retries; Redis loss degrades without losing committed state; graceful shutdown drains in-flight work; a backup restores into an empty environment. |
| M06 | Published artifact versions remain immutable and attributable to exact experiment inputs/outputs; two overlapping booking transactions cannot both commit. |
| M07 | Chat and document events survive reconnect via authoritative pagination; revoked users cannot subscribe or fetch history; managed search returns only authorized team results. |
| M08 | Prompt-injection and forged tool arguments cannot expand retrieval scope; stale/revoked authorization blocks execution; consequential proposals require a current human approval and remain fully auditable. |

## Required negative-space coverage

Every tenant-owned endpoint must include wrong organization, wrong team, removed
membership, missing tenant context, and guessed public-ID cases. Every mutation
must include invalid input, stale version, duplicate/replayed request, database
failure, and timeout/cancellation cases where applicable. Persistence and RLS
properties must use real PostgreSQL; mocks are only for service branching.
