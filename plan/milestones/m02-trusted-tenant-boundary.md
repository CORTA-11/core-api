# M02 — Trusted tenant boundary

| Field | Value |
| --- | --- |
| Status | `in progress` |
| Outcome | Only server-resolved organization and team context can reach tenant data, and PostgreSQL contains mistakes with schema and row-level isolation. |
| Depends on | M01-D01, M01-D03, M01-D04 |
| Release | Security foundation |

**Implementation package:** [M02 master handoff and ordered plan](../implementation/m02/README.md)

**Deliverable playbooks:** [D01](../implementation/m02/d01-split-public-tenant-persistence.md) ·
[D02](../implementation/m02/d02-resumable-tenant-provisioning.md) ·
[D03](../implementation/m02/d03-trusted-tenant-executor.md) ·
[D04](../implementation/m02/d04-team-ownership-force-rls.md) ·
[D05](../implementation/m02/d05-isolation-suite.md)

## Target request-to-database path

```text
authenticated principal + organization public ID + team public ID/slug
  -> control DB membership lookup
  -> opaque trusted OrganizationContext / TeamContext
  -> tenancy.Executor transaction
  -> SET LOCAL trusted search_path/app.user_id/app.team_id
  -> tenant sqlc queries under FORCE RLS
  -> commit/rollback and pool cleanup
```

Handlers and domain services never receive or construct schema names.

## Deliverables

### M02-D01 — Split public and tenant persistence

**Artifacts:** `db/queries/public/`, `db/queries/tenant/`,
`internal/repository/publicdb/`, `internal/repository/tenantdb/`, `sqlc.yml`.

- [x] Move control-plane queries and tenant queries to separate sqlc packages.
- [x] List columns explicitly and add deterministic limits/order to list queries.
- [x] Keep generated files source-derived and update imports without changing
  endpoint behavior in the same change.
- [x] Add compile-time repository interfaces only at service/test boundaries
  that need substitution.

**Acceptance:** `make generate-check`, unit tests, and existing handler behavior
pass; no non-generated SQL query under `db/queries` uses `SELECT *` or
`RETURNING *`.

### M02-D02 — Resumable tenant provisioning and migration fleet

**Artifacts:** next public migrations, tenant migration ledger, `cmd/provision/`
or `cmd/migrate tenant-*`, `internal/tenancy/provisioner.go`.

- [x] Add organization lifecycle fields (`provisioning`, `active`, `failed`,
  `deleting`) and a recorded tenant schema/version/error state.
- [x] Allocate schema names server-side from the stored organization identity;
  never accept them from HTTP or CLI input without registry resolution.
- [x] Create schema, privileges, tenant migration ledger, and all tenant
  migrations as a resumable operation with an advisory lock.
- [x] Extend migration commands to apply/status/retry one tenant and the tenant
  fleet with bounded concurrency and per-tenant results.
- [x] Ensure failed provisioning never exposes the organization as active.

**Acceptance:** integration tests cover new organization success, injected
migration failure, retry, concurrent provision requests, already-current tenant,
and fleet partial failure. Empty-database and upgrade-from-current paths pass.

### M02-D03 — Single tenant executor

**Artifacts:** `internal/tenancy/context.go`, `resolver.go`, `executor.go`, tests;
removal of `internal/service/schema.go` after callers migrate.

- [x] Introduce opaque trusted tenant/team context types constructed only by a
  control-database resolver.
- [x] In one transaction, set an identifier-quoted trusted search path plus
  transaction-local `app.user_id` and `app.team_id` settings.
- [x] Expose organization- and team-scoped execution callbacks that provide the
  tenant sqlc queries; do not expose the raw pool or schema string.
- [x] Guarantee rollback on error/panic/cancellation and verify connection state
  before reuse in tests.

**Acceptance:** unit tests cover resolver failures and transaction outcomes;
real PostgreSQL tests reject unknown schema records, missing settings, cancelled
operations, and malicious schema-like strings.

### M02-D04 — Team ownership and FORCE RLS

**Artifacts:** next tenant migrations and tenant queries.

- [ ] Add public IDs to teams and team-owned entities; keep numeric IDs internal.
- [ ] Add team membership with role and uniqueness constraints linked to the
  authenticated public user identity.
- [ ] Make `tasks.team_id` non-null after backfill and add the access-path index.
- [ ] `ENABLE` and `FORCE ROW LEVEL SECURITY` on every team-owned table, with
  policies driven by transaction-local settings and membership.
- [ ] Use separate owner/migrator/runtime database roles so the runtime role
  cannot create schemas, bypass RLS, or mutate migration ledgers.

**Acceptance:** catalog assertions verify privileges, FORCE RLS, policies,
non-null ownership, and indexes. The runtime role cannot disable RLS, create a
schema, or read a different team.

### M02-D05 — Isolation suite

**Artifacts:** integration tests behind `make test-isolation`.

- [ ] Create at least two organizations, two teams per organization, and users
  with overlapping/non-overlapping membership.
- [ ] Test cross-organization and cross-team reads/writes using production query
  paths and a deliberately unsafe query with no team predicate.
- [ ] Alternate thousands of operations across a deliberately small pgx pool.
- [ ] Cover missing user/team setting, stale membership, rollback, panic, and
  connection cancellation.

**Acceptance:** the M02 demonstration in `verification.md` passes under the
runtime role. Mock-only evidence is not accepted.

## Merge order

M02-D01 → D02 → D03 → D04 → D05. D03 tests may be prepared with D02, but no
handler should be migrated to tenant execution until D04 and its real database
tests exist.

**Implementation links:** M02-D01 [PR #23](https://github.com/CORTA-11/core-api/pull/23),
implementation `3d7a1db`, merge `94b659b`. M02-D02 [PR #24](https://github.com/CORTA-11/core-api/pull/24)
merged at `59d6101`; M02-D03 [PR #25](https://github.com/CORTA-11/core-api/pull/25)
merged at `766ef56`. M02-D04 is active.
