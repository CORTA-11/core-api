# M03-D02 — Revocable server sessions and CSRF

| Field | Value |
| --- | --- |
| Status | `planned` |
| Branch | `feat/m03-d02-server-sessions` |
| PR title | `feat(auth): add revocable browser sessions` |
| Predecessor | M03-D01 merged |
| Dependencies | Local credential verifier and public repository |
| Merge gate | `make check`, unit, integration, race, and generate checks |

## Outcome and security invariants

Successful local authentication creates a revocable opaque browser session based
on [OWASP session-management guidance](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).
The browser receives 256 random bits; PostgreSQL receives only SHA-256 of that
token. A stolen database does not directly yield a usable session secret.

- Session lookup, expiry, revocation, rotation, and password change are
  PostgreSQL-authoritative and transactional.
- Idle expiry is 30 minutes from bounded last-seen state; absolute expiry is 12
  hours from creation and is never extended.
- Session creation and privilege-sensitive changes rotate identifiers. Supplied
  cookie values can never become stored session identifiers.
- Unsafe cookie-authenticated requests require both an approved exact `Origin`
  and a constant-time-valid `X-CSRF-Token` derived for the current session.
- Raw tokens and CSRF values exist only at the HTTP boundary and are never
  persisted, logged, returned by inspection, or accepted in URLs.

## Current state and deficiencies

The application has no session table or revocation. Login returns a bearer JWT
readable by JavaScript; logout, session inspection, idle expiry, rotation, and
CSRF do not exist. Existing CORS does not authorize `X-CSRF-Token`. Password
updates occur through public user CRUD without current-password verification or
session invalidation.

## Scope

In scope:

- Add `public.sessions`: internal BIGINT primary key, unique public UUID, user FK
  with cascade/revocation semantics, unique 32-byte token hash, created and
  last-seen timestamps, absolute expiry, nullable revoked timestamp, and bounded
  normalized user-agent metadata (maximum 256 UTF-8 bytes).
- Add indexes for active token lookup, bounded user session listing, and expiry
  cleanup. Database checks enforce timestamp ordering and metadata bounds.
- Generate tokens only with `crypto/rand`; encode as unpadded base64url. Hash the
  decoded raw bytes with SHA-256 before repository access.
- In production set `__Host-synodus_session` with `Secure`, `HttpOnly`, `Path=/`,
  no `Domain`, and `SameSite=Lax`. Development uses
  `synodus_dev_session`, is non-secure, and is rejected in production.
- Derive CSRF as base64url `HMAC-SHA-256(csrf_secret, "csrf-v1\x00" || raw_token)`.
  Require a dedicated random production secret of at least 32 bytes. Expose the
  derived value only in successful login and current-session JSON responses.
- Authenticate cookies with a single hash lookup that requires non-revoked,
  non-expired state. Coalesce `last_seen_at` writes to at most once per five
  minutes without changing idle correctness.
- Add bounded newest-first session inspection (maximum 100), current logout,
  logout-all, specific-session revocation owned by the current user, and a
  bounded batch cleanup command/job.
- Change password only after verifying the current password and new D01 policy.
  In one transaction update the hash, revoke every other session, revoke the
  current session, and insert a new rotated current session. Return/set the new
  cookie and CSRF only after commit.

Administrative account suspension/revocation hooks may call the same repository
primitive. Public password recovery, remember-me sessions, MFA, and API tokens
are deferred.

## Interfaces, HTTP behavior, and compatibility

`internal/session` exposes a narrow `Manager` accepting D01's user public ID and
returning a one-time `IssuedSession{RawToken, CSRFToken, PublicID, ExpiresAt}`.
Authenticated request context contains only session/user public IDs; raw secrets
do not propagate into service contexts. Repository methods accept token hashes.

Build the v1 auth handlers/router dark for D06:

```text
POST   /api/v1/auth/login
GET    /api/v1/auth/session
DELETE /api/v1/auth/session
GET    /api/v1/auth/sessions
DELETE /api/v1/auth/sessions
DELETE /api/v1/auth/sessions/{session_id}
PUT    /api/v1/auth/password
```

Login rotates any pre-auth cookie by ignoring/revoking it and always issuing a
new random session. Logout is idempotent, clears the cookie with identical
attributes, and returns `204` even when the presented session is already gone.
Session failures become the D04 `401` problem; resource ownership failures for a
specific session are indistinguishable `404`. D06 controls public mounting.

## Test-first matrix

| Initial failing test/check | Expected red result | Passing criterion |
| --- | --- | --- |
| session migration/catalog test | no constraints/indexes/table | fresh and upgrade schemas enforce the complete bounded model |
| token persistence test | raw/encoded token reaches database | only fixed 32-byte hashes are stored; random tokens are unique |
| cookie matrix | production cookie is weak or dev cookie reused | exact production/dev attributes and startup rejection pass |
| expiry/last-seen test | idle or absolute timeout extends incorrectly | 30-minute idle and 12-hour absolute boundaries hold under a fake clock |
| fixation/replay test | supplied/rotated token remains usable | every login/rotation is fresh and predecessor replay returns `401` |
| CSRF matrix | header alone or origin alone succeeds | unsafe requests need both; safe methods do not require token |
| inspection/revocation test | unbounded list or cross-user ID reveals state | maximum 100, stable order, owner-scoped opaque `404` |
| concurrent logout test | races return errors or retain live state | calls are idempotent and no revoked token authenticates |
| password rotation transaction test | partial hash/session state commits | DB failure rolls all back; success revokes others and rotates current atomically |
| cleanup test | one operation scans/deletes without bound | indexed batches stop at configured maximum and preserve active sessions |

Persistence, concurrency, expiry predicates, and failure injection run against
real PostgreSQL. Cookie/CSRF parsing uses handler tests with fixed clock/random
interfaces; production uses only cryptographic randomness.

## Ordered implementation

1. Add failing fresh/upgrade/catalog tests, then add session migration, queries,
   generated code, and bounded cleanup.
2. Add token/hash/randomness and cookie matrix tests, then implement issuance and
   strict configuration validation.
3. Add expiry/last-seen/revocation tests, then implement authenticated lookup,
   inspection, logout-current/all/specific, and cleanup.
4. Add CSRF derivation, origin/header, fixation, replay, and constant-time tests;
   implement middleware without logging secrets.
5. Add password-change rollback/concurrency tests, then implement atomic hash
   update, other-session revocation, and current-session rotation.
6. Build the dark v1 auth handlers, run failure/race regressions, and record
   migration and red/green evidence.

## Atomic green commits

1. `feat(auth): add durable session persistence`
2. `feat(auth): issue bounded browser session cookies`
3. `security(auth): bind csrf protection to sessions`
4. `feat(auth): inspect and revoke active sessions`
5. `security(auth): rotate sessions on password change`
6. `docs(plan): link m03-d02 implementation`

## Verification and acceptance

```bash
make generate-check
make test-unit
make test-race
make test-integration
make check
git diff --check
```

- [ ] Database contains no raw token/CSRF value and enforces declared bounds.
- [ ] Cookie names/attributes cannot cross development and production.
- [ ] Idle, absolute, revoked, rotated, fixed, and replayed sessions fail safely.
- [ ] Unsafe browser mutations require approved origin plus current CSRF token.
- [ ] Inspection and revocation are bounded, user-scoped, and concurrency-safe.
- [ ] Password change and session rotation are one atomic state transition.
- [ ] Database failures produce no partial session/password change.
- [ ] PR records red and green evidence.

## Rollout, rollback, and operations

Apply the additive public migration before deploying session code. Validate the
dedicated CSRF secret and production HTTPS/cookie mode at startup; do not reuse a
JWT, database, or rate-limit secret. Run cleanup in bounded batches and observe
aggregate created/revoked/expired counts without user-agent or token labels.

Until D06, the session router remains unmounted. Rollback may leave the additive
table unused. After D06, rollback is forward-only to a corrected session build;
do not re-enable JWT or unauthenticated routes. Rotating the CSRF secret requires
deliberate invalidation/relogin behavior documented in the runbook.

## Handoff to D03

Provide the authenticated principal context, session lookup/revocation queries,
transaction hooks for mutation-time checks, cookie/CSRF contracts, fake-clock
fixtures, and database evidence. D03 adds current authorization but never places
roles or tenant identifiers in the session.

## Implementation record

**Pull request:** _pending_

**Merge commit:** _pending_

**Red/green evidence:** _pending_
