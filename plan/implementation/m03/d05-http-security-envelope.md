# M03-D05 — HTTP security envelope

| Field | Value |
| --- | --- |
| Status | `planned` |
| Branch | `security/m03-d05-http-envelope` |
| PR title | `security(http): enforce the API security envelope` |
| Predecessor | M03-D04 merged |
| Dependencies | Route inventory, problem contract, session and authorization layers |
| Merge gate | `make test-contract`, unit, integration with real Redis, race, and `make check` |

## Outcome and security invariants

The API boundary has validated exact origins, trusted client identity, bounded
HTTP resources, safe structured logs, security headers, and shared rate limits.
Proxy headers are data only when the direct peer is in configured trusted CIDRs.

- Credentials are allowed only for an exact configured origin; no reflection,
  wildcard, suffix, regex, or insecure production origin is accepted.
- Request/client IP, scheme, and host are derived once from the direct peer and
  validated trusted-proxy chain, then stored as typed context.
- Login and administrative mutation limits use Redis GCRA and HMAC-pseudonymous
  identifiers; Redis is never authorization/session/domain state.
- Redis outage denies login and administrative mutations with a bounded `503`.
  Ordinary authenticated reads/content mutations continue without rate-limit
  state and still require database authorization.
- Headers, problems, and structured logs never expose secrets or attacker-sized
  values.

## Current state and deficiencies

CORS is a hard-coded localhost map and answers any preflight with `204`. The
server has no `ReadHeaderTimeout` or explicit 32 KiB header cap, body limits are
not route-specific, and boundary logging uses chi's basic logger. Forwarding
headers are not governed by trusted proxy CIDRs. pprof mounts under `/debug` on
the same public router when enabled. There is no shared rate limiting or complete
redaction policy.

## Scope

### HTTP and proxy boundary

- Parse an exact origin allowlist at startup. Origins contain scheme, host, and
  optional port only; reject paths, userinfo, fragments, duplicates, `*`, `null`,
  and non-HTTPS production values. Permit explicit localhost HTTP only outside
  production. Emit `Vary: Origin` and preflight-request headers.
- For matched origins, allow credentials and only documented methods/headers,
  including `Content-Type`, `X-CSRF-Token`, `If-Match`, and `Idempotency-Key`.
  Unmatched preflights receive the normal opaque forbidden problem and no CORS
  grant headers.
- Configure `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, and
  `MaxHeaderBytes = 32 << 10`; defaults are 5s, 15s, 30s, 60s respectively and
  production validation forbids zero/unbounded values.
- Assign route body classes in D04's inventory: no-body, auth/password JSON 4 KiB,
  ordinary JSON 64 KiB, and explicitly larger future upload metadata only. Use
  `MaxBytesReader`, reject before decode where possible, and drain only a small
  fixed amount before closing.
- Configure trusted proxy CIDRs. If the direct peer is trusted, parse the
  standardized forwarding chain with a maximum of 10 hops and walk right-to-left
  over trusted hops; otherwise ignore all forwarded values. Reject malformed or
  overlong chains. Never log the raw header.
- Emit stable request ID, trusted client IP prefix (masked, not full address),
  method, route template, status, duration, response bytes, session/user public
  ID only when safe, and typed error class. Do not log query strings, bodies,
  cookies, authorization/CSRF/idempotency values, email, user agent, object keys,
  raw IPs, or internal/database errors.
- Set `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, a minimal
  `Permissions-Policy`, clickjacking protection (`frame-ancestors 'none'` for
  HTML or `X-Frame-Options: DENY`), `Cache-Control: no-store` on auth/problems,
  and HSTS only on validated production HTTPS. CSP must match actual content;
  JSON responses use `default-src 'none'; frame-ancestors 'none'`.

### Redis-backed GCRA

- Use one atomic Redis script/command path with context deadline and bounded
  response parsing. Namespace/version every key and set finite TTLs.
- Derive keys as HMAC-SHA-256 with a dedicated production secret over canonical
  account email or trusted client IP plus bucket name. Store/log neither email
  nor raw IP in Redis keys/values.
- Consume a client-IP login bucket on every attempt: 20 attempts per rolling 15
  minutes, burst 20. Consult the account-failure bucket before verification and
  consume it only after an invalid result: five failures per rolling 15 minutes,
  burst five. Successful authentication deletes the account failure bucket.
- Preserve enumeration resistance: unknown accounts use the same canonical
  keyed bucket, verifier work, response, and clearing rules as known accounts.
- Return the same bounded `429` problem and integer `Retry-After` for either
  login bucket without naming the bucket. Because rate-limit admission precedes
  session authentication, rate-limit administrative mutation classes by trusted
  client IP using reviewed defaults in configuration and the same generic
  response; do not treat an unverified cookie as a user identity.
- If Redis times out/errors, return `/problems/dependency-unavailable` with `503`
  for login and administrative mutations. Do not attempt password verification
  after failed admission. Ordinary authenticated requests skip the limiter and
  continue through PostgreSQL authorization.

### Diagnostics

Remove pprof from the public chi router. When explicitly enabled outside
production, run `http.DefaultServeMux`-free pprof handlers on a separate server
bound to a configured loopback IP and distinct port with the same bounded server
timeouts. Production startup rejects pprof enablement and non-loopback binds.

## Interfaces and middleware contract

`internal/httpx` owns origin, trusted-client, response-header, logging, and body
limit middleware. `internal/ratelimit` owns the Redis GCRA adapter and typed
bucket policy. Both consume D04's route metadata; neither makes authorization
decisions. All failures use D04 problems and carry a request ID.

D06 wires the final order. D05 may exercise a dark v1 router and server harness,
but public compatibility remains unchanged until cutover.

## Test-first matrix

| Initial failing test/check | Expected red result | Passing criterion |
| --- | --- | --- |
| CORS/origin table | wildcard/insecure/suffix origin starts or receives credentials | config rejects; only exact approved origin receives grant/Vary headers |
| header/body tests | oversized input reaches handler | >32 KiB headers and route body excess fail with bounded response/time |
| slow client/server timeout test | partial header/body occupies server indefinitely | configured timeouts terminate and server remains healthy |
| forwarded spoof matrix | untrusted `X-Forwarded-For` controls client identity | direct peer trust and bounded right-to-left parsing select expected IP |
| security-header matrix | success/error/preflight omit policy | headers are consistent and HSTS appears only under production HTTPS |
| log-redaction canary | secrets/raw IP/email/body appear | capture contains route-safe fields and no canary, including panic/error paths |
| real-Redis GCRA boundaries | limits are process-local/off by one | two app instances share exact account/IP windows and `Retry-After` |
| enumeration/rate-key test | email/raw IP stored or response differs | HMAC keys only; known/unknown observable results match |
| Redis outage test | login/admin fails open or all API fails closed | sensitive classes get bounded `503`; ordinary authorized request proceeds |
| pprof topology test | `/debug/pprof` remains public/non-loopback | public router is `404`; diagnostics require explicit loopback config |

Use real Redis for scripts, TTLs, multi-instance limits, success clearing, and
outages. Use actual `http.Server` sockets for header/slow-client/timeout/proxy
tests; mocks cover adapter error branches only.

## Ordered implementation

1. Add configuration/origin/proxy red tests, then implement strict parsing and
   trusted client derivation.
2. Add real-server header/body/timeout tests; apply server and per-route limits.
3. Add success/error/preflight header tests and structured log canaries; implement
   security response middleware and boundary logging/redaction.
4. Add real-Redis GCRA boundary, pseudonymization, multi-instance, clear-success,
   and outage tests; implement the adapter and route classes.
5. Add pprof topology/startup tests and move diagnostics to a loopback-only
   listener.
6. Run contract/failure/race regressions and record defaults, configuration,
   Redis, and red/green evidence.

## Atomic green commits

1. `security(http): validate origins and trusted proxies`
2. `security(http): bound server headers bodies and timeouts`
3. `security(http): add headers and redacted boundary logs`
4. `security(http): rate limit sensitive routes with redis`
5. `security(debug): isolate pprof on loopback`
6. `docs(plan): link m03-d05 implementation`

## Verification and acceptance

```bash
make test-unit
make test-race
make test-integration
make test-contract
make check
git diff --check
```

- [ ] Exact-origin credentialed CORS and startup validation pass all negative cases.
- [ ] Header, body, timeout, proxy-chain, and response headers are bounded/tested.
- [ ] Boundary logs and every error path redact all declared secret classes.
- [ ] Real Redis proves account/IP GCRA limits, success clearing, TTLs, and sharing.
- [ ] Redis outage denies only login/admin mutations as designed.
- [ ] Public router has no pprof path and diagnostic bind is loopback-only/dev-only.
- [ ] PR records red and green evidence.

## Rollout, rollback, and operations

Before deployment, validate exact production web origins, reverse-proxy CIDRs,
HTTPS assumptions, all timeout/body settings, Redis deadlines, and a dedicated
rate-limit HMAC secret. Roll out behind the known proxy topology and compare
aggregate 429/503/timeout counts without identity labels. A proxy mismatch must
fail to the direct peer, not trust attacker forwarding.

Rollback retains no durable state beyond expiring Redis keys. After D06, roll
forward a corrected envelope; never disable origin/CSRF checks, trust all proxies,
serve public pprof, or fail open login/admin because Redis is unavailable.

## Handoff to D06

Provide typed route classes, exact middleware constructors/order constraints,
origin/proxy/timeouts/body configuration, rate-limit keys/policies, diagnostic
listener lifecycle, redaction canaries, and real-server/Redis evidence. D06 only
wires these proven components and removes the old boundary.

## Implementation record

**Pull request:** _pending_

**Merge commit:** _pending_

**Red/green evidence:** _pending_
