# M02-D03 — Trusted tenant resolver and executor

| Field | Value |
| --- | --- |
| Status | `complete` |
| Branch | `refactor/m02-d03-tenant-executor` |
| PR title | `refactor(tenancy): add trusted tenant executor` |
| Predecessor | M02-D02 merged; all active organizations current |
| Dependencies | Public registry/lifecycle, tenant migration currency, split sqlc packages |
| Merge gate | Unit, integration, isolation, race, and generation checks |

## Outcome and security invariants

Only opaque contexts returned by a control-plane resolver can select a tenant
schema or transaction-local identity. Their state is unexported and their zero
values are rejected. Executor callbacks expose only `tenantdb.Queries`, never a
pool, transaction, schema string, or method that can escape the transaction.

Every setup value is server-resolved. Search paths are identifier-safe and
transaction-local; `app.user_id` and `app.team_id` never survive rollback,
commit, cancellation, panic, setup failure, or pool reuse.

## Current repository state and deficiencies

Handlers/services derive schemas via `service.SchemaName`, accept schema strings,
format `SET LOCAL search_path`, and own repeated transaction logic. Team IDs are
numeric request context. The mixed persistence package is removed by D01 and the
registry is trustworthy after D02, but no capability-like execution boundary exists.

## Scope

In scope:

- Add organization/team context types with unexported fields, private constructors,
  explicit validation, and zero-value rejection.
- Resolve organizations only when the authenticated public user exists, has
  membership, the organization is active/current, its canonical registry value
  matches server derivation, and the schema exists.
- Resolve teams from trusted organization context and current membership; D04
  will complete resolution against its final membership model.
- Add `WithinOrganization` and `WithinTeam` callback APIs exposing only tenant queries.
- Begin a transaction, safely set transaction-local search path, `app.user_id`,
  and (for team scope) `app.team_id`, execute, and commit.
- Bound cleanup/rollback for returned errors, panics, cancellation, setup failure,
  and commit failure; preserve panic semantics after cleanup.
- Test unknown schemas, forged schema-like registry values, missing settings,
  cancellation, and reused connections against real PostgreSQL.

Explicitly out of scope: handler/service cutover, JWT enforcement changes, RLS
policies, database role split, API response cleanup, and deleting
`internal/service/schema.go`. That file stays until D04 migrates all callers.

## Interfaces, persistence, commands, and compatibility

The conceptual boundary is:

```go
WithinOrganization(ctx, organizationContext, func(*tenantdb.Queries) error) error
WithinTeam(ctx, teamContext, func(*tenantdb.Queries) error) error
```

Concrete signatures may support typed results without exposing connection state.
Resolvers accept authenticated public-user and public organization/team identity,
then query `publicdb` and the canonical registry. No migration is expected unless
a narrowly scoped database function is necessary and reviewed. No command or
route changes occur; existing code continues using the legacy path until D04.

## Test-first matrix

| Initial failing test | Expected red result | Passing criterion |
| --- | --- | --- |
| zero/forged context unit test | Struct literals or zero values can execute | Only resolver-created valid contexts execute |
| inactive/nonmember resolver test | Registry lookup alone grants context | Active/current org, public user, and membership are all required |
| unknown schema PostgreSQL test | Resolver/executor reaches nonexistent schema | Resolution/execution fails closed before callback work |
| forged schema-like registry test | Malicious identifier changes SQL/search path | Canonical mismatch is rejected; no injected statement/object access |
| missing setting test | Tenant query runs without user/team context | Query fails closed and transaction rolls back |
| callback error/panic test | Transaction or connection remains dirty | Bounded rollback occurs; error/panic propagates correctly |
| cancellation/setup/commit failure test | Cleanup blocks or state leaks | Cleanup is deadline-bounded and connection is safe/discarded |
| pool-reuse test | Next borrower observes prior search path/settings | Reused connection has default state and cannot access prior tenant |

## Ordered implementation

1. Write compile/unit tests for opacity, zero values, and resolver denial cases.
2. Implement organization context and resolution through public identity,
   membership, active/current lifecycle, canonical registry, and schema existence.
3. Add failing transaction outcome tests and implement `WithinOrganization`.
4. Add PostgreSQL injection/missing-setting/cancellation/pool-reuse tests and harden cleanup.
5. Add team context/resolution tests and `WithinTeam`, using the pre-D04 model only as a temporary bridge.
6. Run race/isolation lanes without migrating handlers or deleting legacy schema code.
7. Record implementation and D04 limitations in the handoff.

## Atomic green commits

1. `docs(plan): hand off m02-d02 to m02-d03`
2. `feat(tenancy): resolve trusted organization contexts`
3. `feat(tenancy): execute organization transactions`
4. `security(tenancy): harden executor cleanup`
5. `feat(tenancy): resolve trusted team contexts`
6. `feat(tenancy): execute team transactions`
7. `docs(plan): record m02-d03 implementation`

## Verification and acceptance

Run:

```bash
make generate-check
make test-unit
make test-integration
make test-isolation
make test-race
make check
```

- [x] Context state is opaque; invalid/zero values fail before transaction work.
- [x] Resolver validates identity, membership, lifecycle, canonical registry, and schema existence.
- [x] Callbacks receive only tenant queries and cannot retain raw transaction state.
- [x] Identifier-like attacks and missing settings fail in real PostgreSQL.
- [x] All error, panic, cancellation, setup, and commit paths clean up within a bound.
- [x] Pool reuse leaks no schema or application settings.
- [x] No handler/service uses the new executor yet; legacy schema code remains.

## Migration, rollout, rollback, and operations

This is additive, dark infrastructure with no production-path cutover. Deploying
it should not alter request behavior. Rollback removes unused code normally.
Executor logs may contain public IDs and stable error classes but not schema
names, SQL, or credentials. Cleanup deadlines and pool discard behavior require
metrics with bounded labels. A canonical-registry mismatch is an operator-visible
security fault, never something to auto-correct in the request path.

## Handoff to D04

Provide context/executor APIs, proof of all cleanup paths, the temporary team
resolution assumptions D04 must replace, and a complete inventory of direct
schema/search-path callers. D04 must migrate that entire inventory before
removing `internal/service/schema.go`.

### Temporary D04 limitations

- Team resolution authorizes only a current organization member against a
  parameterized existing team slug. D04 must replace this bridge with current
  team membership and public team identity, with RLS rechecking membership.
- The executor installs `app.team_id`, but no production RLS policy consumes it
  yet. D04 owns `ENABLE`/`FORCE ROW LEVEL SECURITY`, role separation, and the
  production handler/service cutover.
- Existing HTTP selectors, numeric team/task IDs, response contracts, JWT
  behavior, and legacy schema derivation are deliberately unchanged.

### Direct schema/search-path caller inventory

Production application paths D04 must migrate:

- `internal/service/schema.go`: `SchemaName` derives tenant names and
  `setSchema` formats tenant `SET LOCAL search_path` statements.
- `cmd/api/handlers/team.go`: `getTeams` and `createTeam` derive schemas;
  `cmd/api/middleware/teams.go`: `TeamMiddleware` derives a schema and resolves a
  numeric team ID.
- `cmd/api/handlers/task.go`: `getTasks`, `createTask`, `updateTask`, and
  `deleteTask` derive schemas and pass numeric team/task IDs.
- `internal/service/team.go`, `team_cache_impl.go`, and `team_impl.go`: all three
  team operations accept schema strings; the implementation begins its own
  transactions and calls `setSchema`.
- `internal/service/task.go` and `task_impl.go`: all four task operations accept
  schema strings and numeric IDs; the implementation begins its own transactions
  and calls `setSchema`.
- `internal/service/org_impl.go`: `GetOrgs` and `CreateOrg` issue direct public
  `SET LOCAL search_path`; `internal/service/user_impl.go`: `CreateUser` does the
  same; all five operations in `internal/service/org_user_impl.go` do likewise.
- `db/queries/public/orgs.sql` still generates the unused
  `GetSchemaFromID` capability. D04 must remove it when no legacy caller can
  request a raw schema.

Expected non-production exceptions after the cutover are the privileged tenant
migration/adoption scope in `internal/tenancy/reconciler.go` and disposable test
setup in `internal/testsupport/integration.go` and integration tests. Legacy
handler/service tests and mocks that assert `SchemaName` or raw `SET LOCAL`
statements must be replaced together with their production callers.

## Implementation record

**Merged PR:** [#25](https://github.com/CORTA-11/core-api/pull/25)

**Merge commit:** `766ef56`

**Branch commits:**

- Handoff documentation: `35efb4d`
- Organization resolver: `0f89a49`
- Organization executor: `8b17b70`
- Cleanup hardening: `b531b0d`
- Team resolver: `25c1861`
- Team executor and one-connection isolation proof: `b7cfe0d`

**Pre-branch evidence:** `main` fast-forwarded at `59d6101`; a verified custom
PostgreSQL dump was stored at `.cache/backups/pre-m02-d03-59d6101.dump`; public
migrations completed; all three non-deleted organizations reconciled and
`provisioner status --all` reported `active`, version `2`, and `current: true`.

**Test-first evidence:** focused red runs failed on the absent
`OrganizationContext`, `ErrInvalidCallback`, `TeamContext`, and `WithinTeam`
interfaces before their respective slices. The cleanup fault matrix was added as
security characterization after the base executor already implemented detached
rollback; it passed on its first run and was committed separately with the
explicit panic/discard contract rather than manufacturing a failing assertion.

**Green verification:** `make generate-check`, `make test-unit`,
`make test-integration`, `make test-isolation`, `make test-race`, `make check`,
and `git diff --check` passed on 2026-08-22. Integration coverage includes
nonmembership/deleted/stale/missing/malicious resolution, exact organization and
team settings, callback rollback, panic, cancellation, setup race, deferred
commit failure, backend termination and connection discard, retained query
handles, and one-connection cross-organization/team reuse.
