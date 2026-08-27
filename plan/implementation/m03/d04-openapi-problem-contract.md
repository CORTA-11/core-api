# M03-D04 — OpenAPI and problem contract

| Field | Value |
| --- | --- |
| Status | `complete on branch` |
| Branch | `feat/m03-d04-api-contract` |
| PR title | `feat(api): establish the v1 contract` |
| Predecessor | M03-D03 merged |
| Dependencies | D02 auth handlers and D03 authorization errors |
| Merge gate | `make test-contract`, `make check`, unit, and generate checks |

## Outcome and invariants

A hand-maintained OpenAPI 3.1 document is the source of truth for the v1 HTTP
surface. Every running route has a documented request, success response,
security requirement, and [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)
problem response; every documented operation has a handler inventory entry.

- Every error response, including router, middleware, decode, limit, timeout,
  panic, dependency, and handler errors, uses `application/problem+json`.
- Problem `type` values are stable relative URIs under `/problems/`; `detail`
  contains no secrets, existence leak, SQL/schema/object key, or internal error.
- Protected resource lookup cannot distinguish absent from unauthorized.
- Pagination is keyset-based, route/scope-bound, signed, size-limited, and never
  accepts client-provided internal IDs.
- Contract validation has fixed tool/library versions and deterministic output.

## Current state and deficiencies

`.gitignore` excludes `/api`, so there is no versioned contract. Routes mix
`http.Error`, direct JSON, and the partial `internal/httpx` problem writer. The
current problem type defaults to `about:blank`, has no structured field
violations, and is not guaranteed at every boundary. Lists have internal fixed
limits but no public page/cursor contract. No route/OpenAPI drift or request and
response conformance test exists.

## Scope

In scope:

- Remove only the `/api` ignore rule and add `api/openapi.yaml` using OpenAPI
  3.1.x plus JSON Schema 2020-12 semantics.
- Pin a maintained Go validator with demonstrated OpenAPI 3.1 request/response
  support in `go.mod` and tooling configuration. Pin any lint CLI/container by
  exact version or digest. Do not generate handlers unless a measured prototype
  materially reduces drift without weakening chi/middleware architecture.
- Describe only the D06 approved auth, organization, team, and task routes. File,
  registration, user CRUD, JWT, pprof, root, and unversioned paths are absent.
- Extend `httpx.Problem` with stable type, title, status, safe detail, request ID,
  and optional `violations[]` entries containing bounded `field`, `code`, and
  safe `message`. Unknown/internal errors always use the fixed internal detail.
- Use a small closed problem registry, including `/problems/invalid-request`,
  `/problems/unauthenticated`, `/problems/forbidden`, `/problems/not-found`,
  `/problems/conflict`, `/problems/precondition-failed`,
  `/problems/rate-limited`, and `/problems/dependency-unavailable`.
- Map session absence/invalidity/expiry/revocation to `401`; map a known
  operation-level permission failure that probes no protected identifier to
  `403`; map missing and unauthorized protected resource IDs to byte-equivalent
  status/type/title/detail `404` bodies apart from request ID.
- Decode JSON with media-type validation, maximum-one-document semantics,
  unknown-field rejection, and structured violations. Never echo submitted
  values into problems.
- Standardize `page_size`: default 50, range 1–100. Use keyset cursors, never
  offset pagination.
- Encode cursors as versioned, unpadded base64url payload plus HMAC-SHA-256. The
  payload contains route ID, organization/team scope public IDs as applicable,
  normalized sort tuple, direction, and expiry; it contains no internal ID.
  Cap the complete token at 512 bytes, use a dedicated production secret of at
  least 32 bytes, constant-time verify before decode use, and reject cross-route,
  cross-scope, expired, malformed, unknown-version, or bad-signature tokens with
  the same invalid-request problem. Default cursor lifetime is 24 hours.
- Add `make test-contract`, document examples, and enforce route/OpenAPI plus
  generated/query drift in `make check`.

## Interfaces and compatibility

The OpenAPI operation IDs are stable code-review identifiers, not authorization
inputs. A checked-in route inventory records method, chi pattern, operation ID,
authentication, CSRF, permission, body limit class, and rate-limit class. D06's
router is built from or checked against that inventory.

`httpx.WriteProblem` accepts typed application errors and bounded violations,
buffers before headers, and records safe server-side diagnostics separately.
`pagination.Codec` is constructed with one secret/key ID and clock; route/scope
values are supplied by the server. Sort tuples are route-specific typed values
and end in a public UUID tie-breaker.

This PR builds conformance around dark v1 handlers and may convert shared error
primitives, but D06 alone removes legacy routes. No legacy shape is added to the
OpenAPI document.

## Test-first matrix

| Initial failing test/check | Expected red result | Passing criterion |
| --- | --- | --- |
| OpenAPI 3.1 validation | `/api` ignored/no document | pinned validator accepts schema and examples deterministically |
| route inventory drift | handler can appear without contract | missing/extra method-pattern-operation fails with actionable diff |
| request conformance | invalid body reaches handler/ad hoc error | media type/schema/size violations return documented problems |
| response conformance | handler emits undocumented status/shape | every success and error validates against its operation |
| problem writer matrix | router/middleware errors use text/about:blank | all errors use registry type, safe detail, request ID, and content type |
| disclosure pair test | missing and unauthorized IDs differ | responses match except request ID and have equivalent work shape |
| pagination boundary | zero/huge/offset accepted | default 50, maximum 100, deterministic keyset page behavior |
| cursor adversarial/fuzz | token crosses route/scope or panics parser | size/version/MAC/expiry/scope checks reject safely before use |
| examples test | examples drift from schemas | every checked-in success/problem example validates |

Contract tests run against the actual dark v1 router with disposable dependencies
where persistence is required, not a separately mocked route implementation.

## Ordered implementation

1. Add a failing OpenAPI validation target, unignore `/api`, pin the validator,
   and add base components/security/problem schemas.
2. Inventory the approved v1 routes and add failing bidirectional drift checks.
3. Add problem-registry/writer tests and route/middleware fallbacks; standardize
   decode and safe violations.
4. Add cursor unit/fuzz and real-list boundary tests; implement the signed codec
   and page-size normalization.
5. Describe each approved operation, examples, and all responses; run live
   request/response conformance against the dark router.
6. Wire `make test-contract` into checks and record version, drift, red, and
   green evidence.

## Atomic green commits

1. `feat(api): add the openapi 3.1 source contract`
2. `feat(http): standardize rfc 9457 problems`
3. `feat(api): add signed keyset pagination`
4. `test(api): enforce route and contract conformance`
5. `docs(plan): link m03-d04 implementation`

## Verification and acceptance

```bash
make test-contract
make test-unit
make generate-check
make check
git diff --check
```

- [x] OpenAPI 3.1 and every checked-in example pass the pinned validator.
- [x] Route inventory and OpenAPI are bidirectionally complete.
- [x] Every dark-auth error path emits RFC 9457 JSON with safe bounded fields;
  D06 applies the same primitives to organization, team, and task handlers.
- [x] `401`/`403`/opaque `404` semantics match the authorization contract.
- [x] Page size and signed cursor bounds/scope are adversarially tested.
- [x] Live requests and responses conform for all seven existing dark-auth
  operations; the twelve D06 operations have validated schemas and inventory.
- [x] No generated handler layer was added.
- [x] Red and green evidence is recorded below.

## Rollout, rollback, and operations

This is an additive/dark contract PR. Publish no promise that the routes are live
before D06. Treat a contract change after merge as a reviewed API compatibility
change. Configure the cursor secret separately from session, CSRF, JWT, and
rate-limit secrets; startup rejects production defaults. Rotation may accept one
explicit previous key only for at most the 24-hour cursor lifetime.

Rollback may remove dark validation code but must not let D06 proceed without an
equivalent contract gate. Never fix conformance by documenting unsafe accidental
responses or distinguishing protected-resource existence.

## Handoff to D05

Provide the validated OpenAPI file, route inventory, problem registry, body-limit
classes, cursor codec/configuration, live conformance harness, and exact safe
error mappings. D05 applies the boundary envelope without inventing alternative
errors or routes.

## Implementation record

**Pull request:** [PR #33](https://github.com/CORTA-11/core-api/pull/33)

**Merge commit:** _pending_

**Implementation commits:** `9d5dc82` OpenAPI 3.1 source, examples, validator,
and initial inventory; `dbbf47d` closed RFC 9457 problems and dark-router
fallbacks; `0659571` signed keyset pagination and cursor-key configuration;
`592b179` route metadata, drift checks, and disposable auth conformance; this
documentation commit.

**Observed red evidence:**

- `GOCACHE="$PWD/.cache/go-build" go test ./internal/apicontract` first failed
  because media examples used schema-style `$ref` values, then exposed the
  expected closed-page `allOf` conflict. The source now uses OpenAPI Example
  Objects and a composable page-link schema.
- The first Docker-backed `make test-contract` failed with
  `request body not allowed for this request`, exposing an upstream validator
  option that rejected documented bodies. The reusable wrapper now performs
  the missing-body check explicitly and leaves schema validation to kin-openapi.
- The first `make check` reached Staticcheck and failed on `QF1001` in the
  bounded cursor key-ID parser. The parser was simplified without changing its
  accepted alphabet.

**Green evidence:** `make test-contract`; `make test-unit`;
`make generate-check`; network-enabled `make check`; and `git diff --check`
passed on this branch. The contract lane ran all seven dark-auth operations
against disposable PostgreSQL dependencies and validated every success/problem
response. The full check reported no called vulnerabilities and no lint,
diagnostic, generated-code, migration, or query drift.
