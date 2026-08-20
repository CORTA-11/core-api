# M02-D05 — Isolation suite

| Field | Value |
| --- | --- |
| Status | `planned` |
| Branch | `test/m02-d05-isolation-suite` |
| PR title | `test(isolation): prove the m02 tenant boundary` |
| Predecessor | M02-D04 merged to refreshed `main` |
| Dependencies | Runtime role topology, FORCE RLS, trusted production paths |
| Merge gate | Full M02 completion gate and recorded implementation links |

## Outcome and security invariants

An adversarial real-PostgreSQL suite proves that organization schemas, current
team membership, FORCE RLS, transaction-local identity, and pool cleanup compose
correctly through production code. The suite intentionally bypasses application
team predicates once to prove the database remains the final boundary.

Runtime behavior tests never use privileged credentials. Privileged owner,
migrator, or provisioner credentials are limited to fixture/catalog setup and
cannot conceal runtime privilege failures.

## Current repository state and deficiencies

M01 supplies a disposable isolation lane with smoke coverage. D01–D04 will add
the actual boundary, but M02 is incomplete until one suite exercises overlapping
memberships, negative space, unsafe generated SQL, cancellation/panic cleanup,
and repeated pool reuse at meaningful volume. Catalog configuration also needs
executable proof rather than migration-text inspection.

## Scope

In scope:

- Fixture two organizations with two teams each.
- Include a shared user, organization-specific users, and an outsider.
- Prove same-team read/write success and cross-organization/team read/write
  denial through production resolver, executor, service, and generated-query paths.
- Add one bounded generated query deliberately lacking a team predicate.
- Run at least 4,000 alternating operations through a pgx pool capped at two connections.
- Cover missing user/team settings, forged registry values, membership revoked
  after resolution, rollback, panic, cancellation, and subsequent pool reuse.
- Assert catalogs for table ownership, grants, ENABLE/FORCE RLS, policies,
  constraints, and access-path indexes.
- Use privileged credentials only for fixture/migration/catalog setup and runtime
  credentials for all behavioral assertions.
- Record M02 evidence, implementation links, and completion status.

Deferred: performance benchmarking, M03 permission semantics, browser/OIDC flows,
new feature domains, and changing production architecture merely to ease tests.

## Interfaces, persistence, commands, and compatibility

Tests invoke production constructors/resolvers/executors and `publicdb`/`tenantdb`
queries; test-only raw SQL is restricted to fixtures and catalog assertions. The
unsafe query has an explicit small limit and no team predicate by design, with a
comment linking the security test. Add no production migration unless the suite
reveals a defect owned by D04; such a defect must be fixed and explained rather
than hidden in fixtures.

`make test-isolation` remains the public command and must provision both
credential classes without printing secrets. No API compatibility change is planned.

## Test-first matrix

| Initial failing test | Expected red result | Passing criterion |
| --- | --- | --- |
| multi-org/team matrix | Some forbidden production operation succeeds or path is untested | Same-team succeeds; every cross-org/team read/write is empty or denied |
| unsafe generated query | Missing predicate returns all rows | Runtime RLS returns only current team rows within the bound |
| 4,000-operation pool stress | Context occasionally leaks across two connections | All alternations preserve identity/schema/team isolation |
| missing settings | Query executes without one/both settings | Default-deny policy blocks access |
| forged registry value | Resolver trusts schema-like stored value | Canonical validation rejects before tenant work |
| revoked-after-resolution | Stale context retains access | Current membership policy denies read and write |
| rollback/panic/cancellation reuse | Next borrower inherits state or pool stalls | Cleanup completes within bound; next valid/invalid operation behaves correctly |
| catalog contract | Owner/grant/RLS/policy/constraint/index differs unnoticed | Exact expected catalog inventory matches every tenant |
| credential separation | Runtime fixture can perform setup/ledger mutation | Privileged operations fail for runtime role |

## Ordered implementation

1. Define the expected catalog/role inventory from D04 and write failing assertions.
2. Build bounded privileged fixture setup for two organizations, four teams, and overlapping users.
3. Write same-team and cross-boundary production-path read/write tests using runtime credentials.
4. Add the bounded predicate-free generated query and prove RLS contains it.
5. Add missing/forged/stale-context and cleanup fault tests.
6. Add a deterministic 4,000-operation alternating stress loop over a two-connection pool.
7. Run empty and M01-upgrade suites, then every completion command twice where repeatability matters.
8. Record PR/commit links, completion evidence, clean worktree, and change M02 status to complete.

## Atomic green commits

1. `test(isolation): add the multi-tenant rls matrix`
2. `test(tenancy): stress pooled tenant context cleanup`
3. `test(isolation): cover stale context and unsafe queries`
4. `docs(plan): record m02 completion evidence`

## Verification and acceptance

Run:

```bash
make check
make test-unit
make test-race
make test-integration
make test-isolation
make generate-check
git status --short
```

Also run the documented empty-database and M01-upgrade migration paths.

- [ ] Fixture contains two organizations, two teams each, shared/specific/outsider users.
- [ ] Same-team success and all cross-boundary read/write denials use production paths.
- [ ] Predicate-free bounded query is contained by RLS.
- [ ] At least 4,000 alternating operations pass with pool maximum two.
- [ ] Missing, forged, stale, rollback, panic, cancellation, and reuse cases pass.
- [ ] Catalog assertions cover ownership, grants, RLS/FORCE, policies, constraints, indexes.
- [ ] Behavioral connections use only runtime credentials.
- [ ] All full-suite, empty, and upgrade gates pass and the worktree is clean.
- [ ] D01–D05 merged PR/commit links are recorded; M02/dashboard say `complete`.

## Migration, rollout, rollback, and operations

The suite itself adds no production migration. It creates disposable schemas,
roles, and data and must always tear them down through the harness. Test logs
must redact URLs/passwords and keep failure output bounded. CI should serialize
shared Docker resources while the suite itself controls database concurrency.
Rollback is removal of test-only changes, but M02 must not be marked complete if
proof is removed. A production failure discovered here reopens the owning D01–D04
deliverable and its rollout analysis.

## Handoff to M03

Hand off the trusted resolver/executor contract, public identifiers, lifecycle
and role topology, closed stored team roles, production-path fixture builders,
catalog inventory, and final isolation evidence. M03 owns permission mapping,
final organization selection, sessions/OIDC, and default-deny API authorization;
it must preserve this database boundary.

## Implementation record

**Merged PR:** _pending_

**Merge commit:** _pending_
