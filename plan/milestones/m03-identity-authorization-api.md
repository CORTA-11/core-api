# M03 — Identity, authorization, and API contract

| Field | Value |
| --- | --- |
| Status | `in progress` |
| Outcome | Browser requests use local credentials and revocable sessions, explicit database-resolved permissions, a stable v1 contract, and a hardened default-deny HTTP boundary. |
| Depends on | M02 complete; TDR-02 and closed TDR-06 |
| Release | Security foundation |

**Implementation package:** [M03 master handoff and ordered plan](../implementation/m03/README.md)

## Architecture decision

[ADR-005](../decisions/adr-005-local-password-bff-sessions.md) closes TDR-06.
M03 uses local email/password verification followed by opaque BFF sessions.
OIDC, MFA/passkeys, public registration, email recovery, and non-browser API
tokens are future extensions, not M03 dependencies. Any future OIDC integration
must end in the same server-session and database authorization model.

## Deliverables

### M03-D01 — Local password authentication

**Plan:** [decision-complete D01 handoff](../implementation/m03/d01-local-password-authentication.md)

**Artifacts:** public email migration/query changes, `internal/identity/`,
operator user-creation command, seed update.

- [x] Preserve accounts while enforcing canonical case-insensitive unique email;
  abort ambiguous duplicate upgrades without merging.
- [x] Enforce 15–128 Unicode characters, at most 1024 bytes, and no composition
  or periodic-rotation rules.
- [x] Bound Argon2id parameters/concurrency before allocation, perform a dummy
  hash for unknown accounts, return one invalid-credentials result, and rehash
  outdated accepted parameters.
- [x] Create accounts only through an interactive or `--password-stdin` operator
  path; never expose a password in argv, logs, or errors.

**Acceptance:** canonical migration collisions, password-policy boundaries,
hostile encoded hashes, unknown-account work, verifier concurrency/cancellation,
rehash races, CLI redaction, and policy-compliant seeds pass.

### M03-D02 — Server sessions and CSRF

**Plan:** [decision-complete D02 handoff](../implementation/m03/d02-server-sessions-csrf.md)

**Artifacts:** `public.sessions`, generated queries, `internal/session/`, dark v1
auth handlers, cleanup/operator behavior.

- [x] Store only SHA-256 hashes of 256-bit opaque tokens with public UUID, user,
  timestamps, 30-minute idle/12-hour absolute expiry, revocation, and bounded
  user-agent metadata.
- [x] Use the production `__Host-synodus_session` cookie contract and a clearly
  separate non-secure development cookie.
- [x] Derive CSRF from the raw session token with a dedicated HMAC secret; require
  an approved origin plus `X-CSRF-Token` for unsafe cookie requests.
- [x] Add bounded inspection and current/all/specific revocation. Verify current
  password and atomically rotate the current session/revoke others on change.

**Acceptance:** expiry, fixation, replay, CSRF/origin, bounded inspection,
concurrent logout, cleanup, password rotation, and database rollback tests pass.

### M03-D03 — Central authorization

**Plan:** [decision-complete D03 handoff](../implementation/m03/d03-central-authorization.md)

**Artifacts:** organization membership migration, operator owner assignment,
`internal/authorization/`, permission catalog, real-database matrix tests.

- [ ] Safely deduplicate legacy organization memberships, backfill closed
  `member` roles, and never guess an owner.
- [ ] Keep ownerless legacy organizations readable but fail administrative
  mutations closed; atomically make new organization creators owners.
- [ ] Define closed organization/team role-to-permission maps for organization,
  team, task, file, audit, and realtime operations through M05.
- [ ] Wrap M02's trusted resolver/executor and re-read membership/role in every
  protected mutation transaction; organization admin grants no team-content access.

**Acceptance:** exhaustive role × permission, last-owner concurrency, ownerless
upgrade, removed membership, lifecycle, wrong scope, guessed ID, unknown value,
and RLS-backed mutation tests pass.

### M03-D04 — OpenAPI and problem contract

**Plan:** [decision-complete D04 handoff](../implementation/m03/d04-openapi-problem-contract.md)

**Artifacts:** `api/openapi.yaml`, pinned OpenAPI 3.1 validator, RFC 9457 writer,
route inventory, signed cursor codec, `make test-contract`.

- [x] Unignore `/api`; make the hand-maintained 3.1 contract and examples the
  source of truth with bidirectional route inventory checks.
- [x] Return stable relative problem types, safe details, request ID, and bounded
  field violations as `application/problem+json` for every error.
- [x] Use `401` for session failure, `403` for known operation denial, and an
  indistinguishable `404` for missing/unauthorized protected IDs.
- [x] Default pages to 50, cap at 100, and use bounded HMAC-signed keyset cursors
  scoped to route, tenant public IDs, and sort tuple.

**Acceptance:** validator, inventory, examples, live request/response,
problem-disclosure, pagination-boundary, and adversarial cursor tests pass.

### M03-D05 — HTTP security envelope

**Plan:** [decision-complete D05 handoff](../implementation/m03/d05-http-security-envelope.md)

**Artifacts:** validated boundary configuration, middleware/server tests, Redis
GCRA adapter, structured logging, loopback diagnostics listener.

- [ ] Enforce exact origins, credential rules, security headers, 32 KiB header
  cap, server deadlines, and route-specific body limits.
- [ ] Derive trusted client identity only through configured proxy CIDRs and log
  bounded structured fields with complete secret redaction.
- [ ] Rate-limit login by account failures and client-IP attempts using HMAC-keyed
  Redis GCRA; deny login/admin on Redis outage while ordinary authorized traffic continues.
- [ ] Remove pprof from the public router and permit only an explicit loopback
  diagnostic listener outside production.

**Acceptance:** CORS, header, body, timeout, forwarding spoof, redaction,
multi-instance Redis limit/outage, security-header, and pprof topology tests pass.

### M03-D06 — Versioned route cutover

**Plan:** [decision-complete D06 handoff](../implementation/m03/d06-versioned-route-cutover.md)

**Artifacts:** `cmd/api/handlers/v1/`, exact route inventory, legacy deletion,
live contract/isolation suite, browser-style M03 demonstration.

- [ ] Mount the approved auth, organization, team, and task operations under
  `/api/v1`; leave only live/ready health routes outside it.
- [ ] Enforce request ID → trusted client → recovery → logs → envelope → rate
  limit → session → unsafe CSRF → service authorization order.
- [ ] Remove root/public user CRUD-registration, unversioned routes, `X-Org-ID`,
  prototype files, JWT service/middleware/config/dependency, and bearer helpers.
- [ ] Apply migrations, assign every active legacy organization an owner, verify
  all configuration/check lanes, run the browser demonstration, and record links.

**Acceptance:** exact route/order inventory, JWT/header/legacy rejection,
browser login/session/password/revocation flow, authorization negative matrix,
live contract conformance, real PostgreSQL/Redis failure paths, and upgrade
rehearsal pass. File APIs remain unavailable until M05.

## Merge order and cutover

D01 → D02 → D03 → D04 → D05 → D06. Each implementation PR starts from refreshed
`main` after its predecessor merges; none is stacked. D01–D05 are additive or
dark infrastructure. D06 is the single compatibility cutover with no temporary
JWT, unversioned, `X-Org-ID`, registration, or file aliases.

**Implementation links:** planning baseline `4c1145e`; D01 merged in
[PR #30](https://github.com/CORTA-11/core-api/pull/30) at `08bf473`; D02 merged
in [PR #31](https://github.com/CORTA-11/core-api/pull/31) at `b603fb1`; and D03
merged in [PR #32](https://github.com/CORTA-11/core-api/pull/32) at `66a8406`.
D04 is prepared in [PR #33](https://github.com/CORTA-11/core-api/pull/33)
through `592b179`. M03 remains in progress pending D05 and D06.
