# M02-D03 — Trusted tenant resolver and executor

| Field | Value |
| --- | --- |
| Status | `planned` |
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

1. `feat(tenancy): resolve trusted organization contexts`
2. `feat(tenancy): execute organization transactions`
3. `feat(tenancy): execute team transactions`
4. `docs(plan): link m02-d03 implementation`

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

- [ ] Context state is opaque; invalid/zero values fail before transaction work.
- [ ] Resolver validates identity, membership, lifecycle, canonical registry, and schema existence.
- [ ] Callbacks receive only tenant queries and cannot retain raw transaction state.
- [ ] Identifier-like attacks and missing settings fail in real PostgreSQL.
- [ ] All error, panic, cancellation, setup, and commit paths clean up within a bound.
- [ ] Pool reuse leaks no schema or application settings.
- [ ] No handler/service uses the new executor yet; legacy schema code remains.

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

## Implementation record

**Merged PR:** _pending_

**Merge commit:** _pending_
