# M03 — Identity, authorization, and API contract

| Field | Value |
| --- | --- |
| Status | `not started` |
| Outcome | Browser/API requests have a revocable identity, explicit permission, stable versioned contract, and default-deny tenant resolution. |
| Depends on | M02 complete; TDR-02 and TDR-06 |
| Release | Security foundation |

## Deliverables

### M03-D01 — OIDC integration and account identity

**Artifacts:** identity-provider service in local/test Compose,
`internal/identity/`, next public migrations, auth integration tests.

- [ ] Implement authorization-code flow with PKCE, issuer/audience/state/nonce
  validation, bounded discovery/JWKS caching, and explicit timeouts.
- [ ] Store external identities by immutable `(issuer, subject)` linked to a
  local user public ID; do not use email as the stable key.
- [ ] Implement the approved transition for existing password accounts and stop
  exposing password login after the transition window.
- [ ] Keep provider tokens out of logs and browser-readable storage.

**Acceptance:** test login succeeds; bad issuer/audience/state/nonce/signature,
expired code, changed email, duplicate link, and provider outage fail safely.

### M03-D02 — Server sessions and CSRF

**Artifacts:** session migration/query package, `internal/session/`, auth
handlers under `/api/v1/auth`.

- [ ] Store hashed opaque session tokens with user, created/last-seen/absolute
  expiry, revocation, and bounded metadata.
- [ ] Set `Secure`, `HttpOnly`, `SameSite` cookies with the approved name and
  rotation behavior.
- [ ] Enforce origin plus CSRF token checks on state-changing browser requests.
- [ ] Implement `GET /api/v1/auth/session` and idempotent logout/revocation.

**Acceptance:** stolen/expired/revoked/rotated session, missing or mismatched
CSRF, disallowed origin, replayed callback, and concurrent logout tests pass.

### M03-D03 — Central authorization service

**Artifacts:** `internal/authorization/`, membership queries, matrix tests.

- [ ] Define closed organization/team roles and named permissions for all M03-M05
  endpoints.
- [ ] Resolve active organization/team membership before tenant execution and
  return a trusted principal/context only after permission checks.
- [ ] Default deny unknown roles, permissions, lifecycle states, and missing
  context; organization administration does not imply research-content access.
- [ ] Recheck permission inside the mutation transaction where membership or
  object state could change concurrently.

**Acceptance:** table-generated role × permission tests plus removed membership,
disabled org, wrong team, guessed object, and unknown-role cases pass.

### M03-D04 — Versioned public routes

**Artifacts:** `cmd/api/handlers/v1/` or equivalent router grouping, compatibility
tests, deletion of unsafe routes after cutover.

- [ ] Mount all product endpoints beneath `/api/v1` and use public UUIDs/slugs,
  never internal numeric IDs or schema names.
- [ ] Replace `X-Org-ID` schema selection with authenticated route/context
  resolution such as `/api/v1/orgs/{org_id}/teams/{team}`.
- [ ] Protect organization/user administration, remove public pprof, and fix
  current route-parameter mismatches.
- [ ] Apply the TDR-02 compatibility decision and publish a removal point for any
  temporary alias.

**Acceptance:** an endpoint inventory test proves no tenant route bypasses
session, tenant resolution, and authorization middleware; legacy/header-based
selection is absent or explicitly time-bounded.

### M03-D05 — OpenAPI and problem responses

**Artifacts:** `api/openapi.yaml`, generated validation/client artifacts if
adopted, `internal/httpx/problem.go`, `.gitignore` update.

- [ ] Unignore `/api`, define request/response/error/security schemas, and make
  the contract the source for conformance tests.
- [ ] Return `application/problem+json` with stable problem types, status, title,
  detail safe for clients, request ID, and field violations.
- [ ] Normalize not-found/forbidden behavior so identifier guessing reveals no
  protected-resource existence.
- [ ] Cap body/header/page sizes and use signed/opaque keyset cursors.

**Acceptance:** contract tests cover success and every documented error for auth,
organization, team, and task entry points; generated artifacts have zero drift.

### M03-D06 — HTTP security envelope

**Artifacts:** middleware/config tests and deployment headers.

- [ ] Configure exact production origins and credentials behavior; preflight
  never grants an unapproved origin.
- [ ] Add security headers, request/body/header deadlines and limits, trusted
  proxy handling, request IDs, and structured boundary logging.
- [ ] Redact cookies, authorization values, tokens, object keys, and database
  details from logs and problems.
- [ ] Rate-limit sensitive auth and administrative endpoints with bounded local
  or shared state and defined fallback behavior.

**Acceptance:** security-header, CORS, redaction, slow-body, oversized-header,
rate-limit, and timeout tests pass.

## Merge order

D01 and D02 form the identity path; D03 follows the identity model. D04-D06 then
replace the public surface as one reviewed cutover. No M04 mutation ships on an
unversioned or header-selected route.

**Implementation links:** _none yet_.
