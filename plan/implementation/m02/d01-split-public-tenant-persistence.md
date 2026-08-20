# M02-D01 — Split public and tenant persistence

| Field | Value |
| --- | --- |
| Status | `planned` |
| Branch | `refactor/m02-d01-split-persistence` |
| PR title | `refactor(db): split public and tenant persistence` |
| Predecessor | Planning PR `docs: establish test-driven m02 handoff` |
| Dependencies | M01 generation checks and unit-test lane |
| Merge gate | `make check`, `make test-unit`, and `make generate-check` |

## Outcome and security invariants

Control-plane and tenant SQL compile into distinct `publicdb` and `tenantdb`
packages. Code cannot accidentally make tenant queries available through a
public query handle. Query sources use explicit projections, deterministic
ordering, and server-controlled bounds. This is structural preparation only:
routes and response bodies remain compatible.

## Current repository state and deficiencies

`sqlc.yml` currently combines both migration trees and all files in `db/queries/`
into `internal/repository`. Public and tenant models therefore share one package.
Several sources use `SELECT *` or `RETURNING *`; organization, user, and team
lists are unbounded, and not every list has deterministic ordering. Services
depend directly on the broad generated type. Existing handler tests define the
compatibility baseline.

## Scope

In scope:

- Split query sources into `db/queries/public/` and `db/queries/tenant/`.
- Configure two sqlc generation units outputting
  `internal/repository/publicdb/` and `internal/repository/tenantdb/`.
- Qualify control-plane tables with `public.` and use explicit columns everywhere.
- Give every list a stable `ORDER BY` with a unique tie-breaker.
- Accept a server-controlled limit, normalize invalid values, and cap lists at 100.
- Add narrow consumer-owned interfaces only where a service or test substitutes queries.
- Migrate imports/generated code while preserving routes, status codes, and JSON.
- Add an automated source check rejecting wildcard projections/returns and
  list queries without an enforceable bound.

Deferred: lifecycle fields, schema allocation, migration ledgers, executor
contexts, RLS, role separation, authentication changes, and API redesign.

## Interfaces, persistence, commands, and compatibility

`publicdb.New(db)` exposes control-plane queries; `tenantdb.New(db)` exposes
tenant queries. Any interface is defined beside its consumer and includes only
the methods that consumer invokes. SQL sources move but migrations do not change.
`make generate` and `make generate-check` cover both packages. Add the query-rule
checker to `make check`. Existing endpoints and response fields are unchanged,
including known legacy fields that D04 will remove.

## Test-first matrix

| Initial failing test/check | Expected red result | Passing criterion |
| --- | --- | --- |
| sqlc configuration/package test | Only the mixed `repository` package exists | Separate `publicdb` and `tenantdb` generation units and imports compile |
| generated drift fixture | Moving/changing sources leaves stale generated output | `make generate-check` detects drift for either package |
| query-source rule fixtures | Existing `SELECT *`, `RETURNING *`, or unbounded list passes | Checker names the file/query and rejects all three patterns |
| compile test across consumers | Imports still require the mixed package | Build succeeds with only the appropriate generated package available |
| handler regression tests | Import/model changes alter status or JSON behavior | Existing organization, team, task, user, and file handler tests pass unchanged |
| list-bound repository test | Caller can request zero, negative, or more than 100 rows | Server chooses a valid limit and never returns more than 100 in stable order |

## Ordered implementation

1. Add failing structure and drift checks for two sqlc packages.
2. Split sources/configuration, regenerate, and migrate imports without behavior changes.
3. Add failing query-rule fixtures and wire the checker into `make check`.
4. Replace wildcard projections/returns with explicit columns.
5. Add failing list limit/order tests, then implement deterministic ordering and the 100-row cap.
6. Introduce only the interfaces required by tests/consumers and rerun handler regressions.
7. Update milestone links and capture red/green evidence in the PR.

## Atomic green commits

1. `refactor(db): split public and tenant sqlc packages`
2. `fix(db): bound and order repository list queries`
3. `build(db): enforce SQL query invariants`
4. `docs(plan): link m02-d01 implementation`

Each commit includes its observed-red test and green implementation; no commit
may intentionally leave the branch failing.

## Verification and acceptance

Run:

```bash
make generate-check
make test-unit
make check
git diff --exit-code
```

- [ ] Two generated packages have disjoint source/schema responsibility.
- [ ] No hand-edited generated file exists.
- [ ] No query source contains `SELECT *` or `RETURNING *`.
- [ ] Every list is deterministically ordered and bounded to at most 100.
- [ ] Public tables are explicitly qualified.
- [ ] Existing route and response behavior is unchanged.
- [ ] PR records red failures and green commands.

## Migration, rollout, rollback, and operations

There is no database migration or runtime rollout sequencing. Generated package
renames are a source compatibility change internal to the repository and must
land atomically with imports. Rollback is a normal Git revert of the full PR;
do not retain two competing generated packages. CI/tool caches may be cleared if
old generated paths survive locally.

## Handoff to D02

D02 begins only after this PR merges and `main` regenerates cleanly. Provide the
final public/tenant package paths, list-limit policy, query checker location, and
any narrow interfaces D02 must use. D02 must not rejoin the packages.

## Implementation record

**Merged PR:** _pending_

**Merge commit:** _pending_
