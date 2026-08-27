# M03 implementation package — local identity, authorization, and API contract

| Field | Value |
| --- | --- |
| Status | `active` |
| Milestone | [M03 — Identity, authorization, and API contract](../../milestones/m03-identity-authorization-api.md) |
| Predecessor | [M02 — Trusted tenant boundary](../../milestones/m02-trusted-tenant-boundary.md), merged in PR #27 at `56d0a6d` |
| Planning baseline | `main` at `4c1145e` |
| Planning branch | `docs/m03-test-driven-handoff` |
| Planning PR | `docs: establish test-driven m03 handoff` |
| Implementation order | D01 → D02 → D03 → D04 → D05 → D06 |

## Handoff from M02

M02 provides separate public/tenant repositories, resumable tenant migrations,
the trusted resolver/executor, public UUID boundaries, closed team roles,
`FORCE ROW LEVEL SECURITY`, production database-role fixtures, and adversarial
isolation evidence. Those are inputs to M03, not seams to bypass. Authorization
must wrap the resolver/executor, and protected mutations must re-read current
membership and role in their transaction.

The current API still issues 24-hour bearer JWTs from a public password-login
route, exposes public user CRUD, uses `X-Org-ID`, has no organization roles or
server sessions, mounts pprof on the public router, and lacks a versioned
contract. M03 replaces that surface in one final cutover after building and
testing the supporting layers dark.

## Shared invariants

- Browser authentication uses local email/password verification followed by a
  revocable opaque server session; authorization facts never live in a token.
- Passwords, raw session tokens, CSRF secrets, cookies, rate-limit identifiers,
  and database details never appear in logs, errors, audit metadata, or CLI
  arguments.
- Organization and team role names are closed; unknown role, permission,
  lifecycle, membership, or tenant context always denies.
- Organization administration grants no implicit team-content permission.
- Client identifiers are public UUIDs. Missing and unauthorized protected
  resource IDs produce indistinguishable `404` problems.
- Cookie-authenticated unsafe requests require an approved exact `Origin` and a
  valid `X-CSRF-Token` bound to the current raw session token.
- Lists, request bodies, headers, metadata, hash work, database operations,
  Redis operations, and concurrency have explicit bounds.
- PostgreSQL remains the authority for accounts, sessions, memberships, roles,
  and authorization. Redis is used only for rate-limiting state in M03.
- Only `/health/live` and `/health/ready` remain outside `/api/v1` after D06.

## Test-driven delivery rule

For each behavior slice, add the smallest proof first, run it, and record the
expected failure. Commit that test with its implementation only after the
focused test and relevant regression lanes pass. PR descriptions record the
exact red command/failure and green commands/results. Red-only commits do not
belong on shared branches.

Real PostgreSQL proves migrations, uniqueness, sessions, transactional rotation,
membership concurrency, role changes, and tenant permission behavior. Real Redis
proves GCRA limits and outage behavior. Mocks prove only bounded service branches.
Security and timing tests assert observable classes and work bounds rather than
fragile wall-clock equality.

## Ordered seven-PR plan

| Order | Branch | Pull request | Plan | Starts after |
| ---: | --- | --- | --- | --- |
| 0 | `docs/m03-test-driven-handoff` | `docs: establish test-driven m03 handoff` | This package | M02 merged at `56d0a6d`; baseline `4c1145e` |
| 1 | `security/m03-d01-password-authentication` | [PR #30](https://github.com/CORTA-11/core-api/pull/30), merged at `08bf473` | [D01](d01-local-password-authentication.md) | Complete |
| 2 | `feat/m03-d02-server-sessions` | [PR #31](https://github.com/CORTA-11/core-api/pull/31), merged at `b603fb1` | [D02](d02-server-sessions-csrf.md) | Complete |
| 3 | `security/m03-d03-authorization` | [PR #32](https://github.com/CORTA-11/core-api/pull/32), merged at `66a8406` | [D03](d03-central-authorization.md) | Complete |
| 4 | `feat/m03-d04-api-contract` | implementation complete through `592b179`; PR pending | [D04](d04-openapi-problem-contract.md) | Complete on branch |
| 5 | `security/m03-d05-http-envelope` | `security(http): enforce the API security envelope` | [D05](d05-http-security-envelope.md) | D04 merged |
| 6 | `refactor/m03-d06-api-v1-cutover` | `refactor(api): cut over to authenticated v1 routes` | [D06](d06-versioned-route-cutover.md) | D05 merged and deployment gates pass |

Implementation PRs are not stacked. After each predecessor merges, switch to
`main`, pull with `--ff-only`, and create the next branch. Each PR owns one
deliverable and records its merged PR and merge commit in its plan.

## Cross-deliverable and compatibility decisions

- [ADR-005](../../decisions/adr-005-local-password-bff-sessions.md) closes
  TDR-06: local passwords plus BFF sessions are the M03 identity architecture.
  OIDC, MFA/passkeys, public registration, email recovery, and non-browser API
  tokens are future work, not M03 dependencies.
- D01–D05 add migrations, services, commands, contract, or middleware behind the
  existing surface or an unmounted v1 router. D06 is the only public compatibility
  cutover.
- D01 preserves current accounts. It aborts a case-folding migration if two
  existing emails canonicalize to the same value; it never guesses or merges.
- D02 owns browser-session and CSRF primitives. D03 owns permissions and current
  database membership. Neither trusts browser claims for roles or tenant IDs.
- D04 owns the hand-maintained OpenAPI source, RFC 9457 problems, and cursor
  format. D05 owns boundary policy and dependency degradation. D06 wires the
  already-proven pieces in the specified middleware order.
- There are no temporary unversioned, `X-Org-ID`, bearer-JWT, registration, or
  file-route aliases. Existing JWTs become invalid at D06.
- M05 replaces prototype filename-key file authorization and unbounded transfer;
  therefore D06 deliberately exposes no file API.
- This planning PR does not change README runtime examples. D06 updates them only
  when the running API matches the documented routes.

## Merge and deployment gates

Every deliverable must pass its focused tests, `make check`, `make test-unit`,
and generated/query drift checks. Add `make test-race` for concurrency changes,
real PostgreSQL integration/isolation lanes for D01–D03 and D06, real Redis tests
for D05, and `make test-contract` from D04 onward.

Before D06 deployment: apply public migrations; verify every active legacy
organization has at least one owner; validate session, CSRF, origin, proxy, and
rate-limit secrets/configuration; run unit, race, integration, isolation,
contract, and full `make check` lanes; and confirm generated/query drift is clean.

Rollback retains additive database state and rolls application code forward to
a corrected session build. It must not restore unauthenticated routes,
`X-Org-ID`, or role-bearing JWT authorization. If cutover cannot be made safely,
hold D06 rather than publishing mixed authentication modes.

## Completion and handoff discipline

D01–D05 update their implementation records after merge. D06 records every PR,
migration rehearsal, configuration check, browser-style demonstration, and CI or
disposable-environment evidence; then it marks M03 and the dashboard complete.
Any failed acceptance proof reopens the deliverable that owns the behavior.

M04 receives the authenticated principal/session API, permission catalog,
trusted authorization/executor composition, OpenAPI/problem conventions,
signed keyset cursor codec, exact route inventory, and boundary middleware. M05
receives explicit Redis degradation, telemetry redaction, and no-file-route
constraints.
