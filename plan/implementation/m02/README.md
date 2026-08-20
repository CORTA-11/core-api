# M02 implementation package — trusted tenant boundary

| Field | Value |
| --- | --- |
| Status | `active` |
| Milestone | [M02 — Trusted tenant boundary](../../milestones/m02-trusted-tenant-boundary.md) |
| Predecessor | [M01 — Reproducible baseline](../../milestones/m01-reproducible-baseline.md), merged in PR #21 at `f2ee418` |
| Planning branch | `docs/m02-test-driven-handoff` |
| Planning PR | `docs: establish test-driven m02 handoff` |
| Implementation order | D01 → D02 → D03 → D04 → D05 |

## Handoff from M01

M01 established deterministic local and CI commands, a disposable integration
environment, typed startup configuration, bounded lifecycle behavior, and shared
HTTP/test primitives. Its local verification passed `make check`, unit, race,
integration, and isolation lanes, and the work merged through PR #21.

M02 may rely on those commands and fixtures, but it must extend them rather than
weaken them. The current API still derives organization schemas in services and
handlers, mixes public and tenant sqlc output, exposes internal identifiers, and
has no database-enforced team boundary. M02 closes that gap before M03 builds
identity and authorization behavior on top of it.

## Invariants shared by every deliverable

- Clients never supply or derive a PostgreSQL schema name.
- Organization schemas are allocated from a server-generated, stored public UUID.
- Numeric database IDs and schema names never cross the external API boundary.
- Tenant queries execute only through an opaque, server-resolved context.
- Team-owned rows are protected by current membership and `FORCE ROW LEVEL SECURITY`.
- Runtime credentials cannot create schemas, bypass RLS, or mutate migration state.
- Every query, fleet operation, retry, wait, and concurrency fan-out has an explicit bound.
- Real PostgreSQL tests prove persistence, migration, role, transaction, and RLS properties.

## Test-driven delivery rule

Each behavior slice starts with the smallest test capable of proving its invariant.
The author must run it and record the expected failure before implementing the
slice. The implementation and its test are committed together only after the
focused test and relevant regression lane are green. A red-only commit must not
be pushed to a shared branch. Each PR description records the exact red command,
the observed failure, and the green commands/results.

Mocks may prove service branching; they cannot prove SQL, migrations, privileges,
RLS, locks, transaction cleanup, or pool reuse.

## Ordered six-PR plan

| Order | Branch | Pull request | Plan | Starts after |
| ---: | --- | --- | --- | --- |
| 0 | `docs/m02-test-driven-handoff` | `docs: establish test-driven m02 handoff` | This package | M01 merged; merge before D01 starts |
| 1 | `refactor/m02-d01-split-persistence` | `refactor(db): split public and tenant persistence` | [D01](d01-split-public-tenant-persistence.md) | Planning PR merged |
| 2 | `feat/m02-d02-tenant-provisioning` | `feat(tenancy): make tenant provisioning resumable` | [D02](d02-resumable-tenant-provisioning.md) | D01 merged |
| 3 | `refactor/m02-d03-tenant-executor` | `refactor(tenancy): add trusted tenant executor` | [D03](d03-trusted-tenant-executor.md) | D02 merged and all active tenants current |
| 4 | `security/m02-d04-team-rls` | `security(rls): enforce trusted team ownership` | [D04](d04-team-ownership-force-rls.md) | D03 merged |
| 5 | `test/m02-d05-isolation-suite` | `test(isolation): prove the m02 tenant boundary` | [D05](d05-isolation-suite.md) | D04 merged and runtime-role deployment path exists |

No implementation PR is stacked. After each predecessor merges, switch to
`main`, pull with `--ff-only`, and create the next branch. Do not combine work
from multiple deliverables in one branch.

## Cross-deliverable decisions

- D01 separates generated persistence without changing routes or response shapes.
- D02 owns lifecycle, schema allocation, migration ledgers, and fleet operations.
- D03 creates trusted contexts/execution but deliberately does not cut handlers or
  services over; `internal/service/schema.go` remains temporarily.
- D04 establishes ownership, roles, RLS, JWT enforcement, and completes the
  production-path cutover before deleting direct schema manipulation.
- D05 is adversarial proof and completion evidence, not a place to introduce new
  production architecture.
- Stored role names are closed in D04, but role-to-permission mapping remains M03 work.
- `X-Org-ID` may remain through D04 only as an organization public-ID selector;
  it is never trusted as a schema or authorization assertion.

## Documentation and completion discipline

Each deliverable updates its plan placeholder with the merged PR and merge commit.
D05 additionally records all completion commands, links implementation evidence
from the milestone, changes M02 to `complete`, and updates the delivery dashboard.
If acceptance evidence reveals a production defect, fix it in the deliverable
that owns the behavior; do not relax the test.
