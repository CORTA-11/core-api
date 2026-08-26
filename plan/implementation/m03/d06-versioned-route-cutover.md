# M03-D06 — Versioned route cutover

| Field | Value |
| --- | --- |
| Status | `planned` |
| Branch | `refactor/m03-d06-api-v1-cutover` |
| PR title | `refactor(api): cut over to authenticated v1 routes` |
| Predecessor | M03-D05 merged and deployment prerequisites satisfied |
| Dependencies | D01–D05 implementation/evidence and current active-org owner inventory |
| Merge gate | all check, unit, race, integration, isolation, contract, and browser demonstration lanes |

## Outcome and security invariants

The running API exposes one authenticated, versioned browser surface. Only
Kubernetes-style liveness/readiness remain outside `/api/v1`; no prototype route
or selector bypasses sessions, CSRF, authorization, contract, or boundary policy.

- Middleware order is fixed: request ID → trusted client resolution → recovery →
  structured logging → security headers/CORS → rate limiting → session
  authentication → CSRF for unsafe methods → handler/service authorization.
- Route metadata can skip session/CSRF only for the explicitly public login and
  health behavior; it cannot skip trusted client, recovery, logs, or envelope.
- Organization/team/task identifiers are route public UUIDs resolved through the
  D03 authorizer and M02 trusted resolver/executor.
- No file API ships before M05 replaces filename-key authorization and unbounded
  streaming.
- There is no dual-mode compatibility window. Existing JWTs become invalid when
  this revision is deployed.

## Current state and deficiencies

Handlers live directly in `cmd/api/handlers`, with root greeting, public
organization and user CRUD/registration, partially JWT-protected unversioned
team/task/file routes, `X-Org-ID`, mismatched parameter names, and optional
public-router pprof. JWT service/config/dependency and bearer-token test helpers
remain wired. D01–D05 deliberately build the replacement mostly dark, so M03 is
not complete until this PR removes every old entry point and proves the composed
browser flow.

## Approved route inventory

Move product handlers beneath `cmd/api/handlers/v1` and mount exactly:

```text
POST   /api/v1/auth/login
GET    /api/v1/auth/session
DELETE /api/v1/auth/session
GET    /api/v1/auth/sessions
DELETE /api/v1/auth/sessions
DELETE /api/v1/auth/sessions/{session_id}
PUT    /api/v1/auth/password

GET    /api/v1/orgs
POST   /api/v1/orgs
GET    /api/v1/orgs/{org_id}
PATCH  /api/v1/orgs/{org_id}
DELETE /api/v1/orgs/{org_id}
POST   /api/v1/orgs/{org_id}/restore

GET    /api/v1/orgs/{org_id}/teams
POST   /api/v1/orgs/{org_id}/teams

GET    /api/v1/orgs/{org_id}/teams/{team_id}/tasks
POST   /api/v1/orgs/{org_id}/teams/{team_id}/tasks
PATCH  /api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}
DELETE /api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}
```

Outside that prefix, mount only:

```text
GET /health/live
GET /health/ready
```

The M03 task handlers preserve only behavior supportable by the current task
model; M04 adds full get/move/concurrency/idempotency semantics and their routes.
Every mounted operation must already exist in D04 OpenAPI and inventory.

## Scope and removals

In scope:

- Relocate/adapt handlers into the v1 package, keep transport parsing there, and
  route all business decisions through D01–D03 services.
- Build the router from explicit public/authenticated groups while retaining the
  global middleware prefix in the invariant order.
- Replace organization selection with `{org_id}` and team selection with
  `{team_id}` public UUID resolution; handlers never read schema names or numeric
  IDs from clients.
- Delete the root greeting, `/users` CRUD/login/registration, `/orgs`, `/teams`,
  `/{team}/tasks`, `/{team}/files`, `X-Org-ID`, prototype file handlers/routes,
  public-router debug routes, and every transitional alias.
- Delete JWT middleware/service/configuration, `JWT_SECRET`, JWT dependency,
  claims, token generation/verification, and bearer-token test helpers. Remove
  the D01 legacy adapter after all consumers move to sessions.
- Update `.env.example`, Compose/Make wiring, repository README runtime examples,
  and operator notes to match actual local session login and CSRF use. Never put
  a real password/session/CSRF value into tracked examples.
- Add a browser-style demonstration that maintains a cookie jar, obtains CSRF
  from login/session JSON, exercises allowed reads/mutations, and proves expiry,
  revocation, bad origin/CSRF, removed membership, insufficient permission,
  guessed IDs, and legacy path/header/JWT rejection.

No file compatibility route, bearer alias, header-selected alias, public account
creation, or temporary redirect is in scope.

## Interfaces, composition, and error behavior

Login is public but still uses trusted client resolution, CORS, body limits,
redaction, and both D05 rate-limit buckets; on success it creates D02 state.
Authenticated safe requests require a current session. Authenticated unsafe
requests additionally require D02 origin/CSRF before service authorization.
Organization/team/task decisions use D03, and database work uses the M02
resolver/executor. D04 maps all failures to contract problems.

Unknown routes/methods, including removed prototype paths, use bounded RFC 9457
`404`/`405` responses without advertising hidden routes. Health responses reveal
only the existing bounded component status policy and never accept credentials.

## Test-first matrix

| Initial failing test/check | Expected red result | Passing criterion |
| --- | --- | --- |
| exact route inventory | old/root/file/user/header route remains | only approved methods/patterns plus two health routes are reachable |
| middleware trace test | auth/CSRF/logging order differs or can be bypassed | marker sequence exactly matches invariant for every route class |
| anonymous/session matrix | protected operation reaches handler | absent/expired/revoked session returns documented `401` before authorization |
| CSRF/origin matrix | unsafe cookie request passes one check | both approved origin and current token are required; safe method remains usable |
| authorization HTTP matrix | removed/low role crosses scope or leaks ID | `403` only for known operation; protected probes share opaque `404` |
| legacy rejection table | JWT, `X-Org-ID`, old paths, file route still work | all are absent and headers cannot select tenant or identity |
| dependency/build test | JWT package/config/module still required | source/dependency/config/tests contain no runtime JWT path |
| contract live test | mounted behavior drifts from OpenAPI | every actual success/error validates; no undocumented live operation |
| browser demonstration | cookie/CSRF/rotation flow cannot complete | full login → org/team/task → password/session revoke flow and negatives pass |
| deployment upgrade rehearsal | ownerless/config/migration gap discovered late | preflight blocks; fully prepared M02 database upgrades and serves v1 safely |

All authorization/tenant/rotation paths run through real PostgreSQL with runtime
roles. Rate limits/outage use real Redis. HTTP proof uses the actual server and
cookie jar, not direct handler calls alone.

## Ordered implementation

1. Add failing exact-route and middleware-order tests against the current router.
2. Create `handlers/v1`, mount the approved dark handlers, and compose D01–D05
   middleware/services through one application wiring path.
3. Add legacy rejection tests, then delete root/public user/unversioned/header/file
   routes and old handlers with no compatibility aliases.
4. Add build/config tests, then remove JWT middleware/service/config/env/module
   dependency and bearer helpers.
5. Run live OpenAPI conformance, real PostgreSQL/Redis isolation and outage
   matrices, and the browser-style demonstration.
6. Rehearse fresh/M02 upgrade and cutover/forward-fix procedure; update README
   runtime examples and operational notes now that behavior exists.
7. Record all M03 PRs/commits/evidence in package, milestone, verification, and
   dashboard; mark M03 complete only after every gate passes.

## Atomic green commits

1. `refactor(api): compose authenticated v1 handlers`
2. `refactor(api): remove prototype routes and selectors`
3. `refactor(auth): remove bearer jwt authentication`
4. `test(api): prove the m03 browser cutover`
5. `docs(runtime): document authenticated v1 usage`
6. `docs(plan): record m03 completion evidence`

## Verification and acceptance

```bash
make generate-check
make test-unit
make test-race
make test-integration
make test-isolation
make test-contract
make check
git diff --check
```

- [ ] Only the approved v1 inventory and two health routes are reachable.
- [ ] Middleware order is exact and every skip is explicitly tested.
- [ ] Browser login/session/CSRF/password/revocation behavior works end to end.
- [ ] Role, ownerless, removed membership, guessed ID, and cross-scope cases deny.
- [ ] JWT, `X-Org-ID`, public user CRUD, file routes, root, and public pprof are gone.
- [ ] OpenAPI and every actual success/error response conform with no drift.
- [ ] Fresh and M02 upgrade/cutover prerequisites are rehearsed.
- [ ] All PR, migration, command, and demonstration evidence is linked.

## Rollout, rollback, and operations

Before deploy: apply public migrations; ensure all active tenant migrations are
current; assign and verify at least one owner for every active legacy
organization; validate password-hash concurrency, session/CSRF/cursor/rate-limit
secrets, exact origins, trusted proxies, Redis, HTTPS cookie mode, and diagnostic
disablement; run every merge-gate lane; and notify prototype users that JWTs and
old routes terminate at cutover.

Deploy migration-compatible code as one application revision. Existing JWTs
immediately fail and users log in to receive sessions. Observe only aggregate
login, session, `401`/`403`/`404`, 429/503, and latency metrics.

Database changes from D01–D03 are additive and retained. Rollback means roll
forward to a corrected session-authenticated build. It must not restore public
registration/CRUD, unauthenticated or unversioned product routes, `X-Org-ID`,
role-bearing JWTs, prototype files, or public pprof. If prerequisites fail, do
not cut over.

## Handoff to M04 and M05

M04 receives the exact authenticated v1 conventions, typed permissions,
transaction-time authorization, OpenAPI/problem/cursor utilities, route metadata,
and browser/tenant fixtures. It extends task/team semantics without reopening
authentication or pagination format.

M05 receives no live file API: it must add metadata-backed authorization and
bounded multipart transfer before exposing one. It also inherits Redis outage
classification and telemetry/log redaction boundaries.

## Implementation record

**Pull request:** _pending_

**Merge commit:** _pending_

**M03 completion evidence:** _pending_
