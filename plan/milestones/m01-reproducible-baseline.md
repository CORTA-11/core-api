# M01 — Reproducible baseline

| Field | Value |
| --- | --- |
| Status | `in progress` |
| Outcome | A fresh checkout has deterministic checks, disposable integration dependencies, validated startup, and bounded process lifecycle. |
| Depends on | None |
| Release | Engineering baseline |

## Current starting point

`go test ./...` passes with a writable Go cache. The repository already has a
CI workflow and Make targets, but `Makefile` unconditionally includes `.env`, CI
uses a floating `gosec@latest`, generated-code drift is unchecked, and tests do
not exercise real infrastructure. Startup reads environment variables directly,
creates a MinIO bucket as a side effect, exposes pprof, and has no graceful
shutdown or readiness checks.

## Deliverables

### M01-D01 — Canonical non-mutating commands

**Artifacts:** `Makefile`, `.env.example`, optional pinned tool manifest.

- [x] Replace unconditional `include .env` with optional runtime-only loading.
- [x] Add `check`, `test-unit`, `test-race`, `test-integration`, `test-isolation`,
  `generate`, and `generate-check` targets; keep formatting fixes separate.
- [x] Pin sqlc, golangci-lint, gosec, govulncheck, and migration tooling used by
  protected checks.
- [x] Make `make check` run without services, `.env`, or secrets and leave the
  worktree unchanged.

**Acceptance:** from a fresh checkout, `make check && make test-unit` passes;
`git diff --exit-code` remains clean. A deliberately stale sqlc file makes
`make generate-check` fail.

### M01-D02 — CI parity

**Artifacts:** `.github/workflows/ci.yml`, tool version/pin files.

- [x] Make pull-request CI call the M01-D01 targets rather than duplicate shell
  commands.
- [x] Add build, generated drift, migration-name/up-down pairing, race, secret,
  and dependency review jobs at the appropriate trigger.
- [x] Pin action and scanner versions; remove `@latest` installation.
- [x] Upload test output only when useful for diagnosing a failure; never upload
  environment files or secrets.

**Acceptance:** fixtures for bad format, stale generation, a missing down file,
and a failing test each fail the named job; a clean branch passes locally and in
CI using the same commands.

### M01-D03 — Disposable integration environment

**Artifacts:** `docker-compose.test.yaml`, `internal/testsupport/`, integration
test packages, Make targets.

- [x] Start isolated PostgreSQL, Redis, and MinIO instances on collision-safe
  ports with health checks and unique test credentials.
- [x] Apply public migrations, create tenant schemas, seed minimal fixtures, and
  tear down volumes after the suite.
- [x] Provide helpers for database cleanup, MinIO bucket cleanup, Redis flush,
  and bounded service readiness.
- [x] Run real adapter tests separately from fast unit tests.

**Acceptance:** `make test-integration` succeeds twice in succession on a clean
machine, leaves no persistent named test volumes, and fails clearly when a
dependency never becomes healthy.

### M01-D04 — Typed configuration and process lifecycle

**Artifacts:** `internal/config/`, `cmd/api/main.go`, `cmd/api/handlers/health.go`,
MinIO/Redis constructors and tests.

- [x] Parse all settings once into typed configuration with units, safe
  development defaults, production validation, and redacted errors.
- [x] Return constructor/startup errors instead of calling `log.Fatal` in
  adapters; bucket creation becomes an explicit bootstrap action.
- [x] Add `/health/live` and dependency-aware `/health/ready` endpoints.
- [x] Handle `SIGINT`/`SIGTERM`, stop accepting requests, drain with a deadline,
  close adapters, and return a non-zero exit code on startup/runtime failure.
- [x] Disable pprof by default and mount it only when an explicit development
  setting is enabled.

**Acceptance:** table tests cover missing/invalid config without printing secret
values; process tests prove readiness transitions and bounded shutdown; pprof is
404 under the default configuration.

### M01-D05 — Shared HTTP and test primitives

**Artifacts:** `internal/httpx/`, handler tests, `plan/verification.md` command
implementations.

- [x] Add bounded JSON decoding, response encoding, request IDs, and a typed
  application-error adapter usable by later API handlers.
- [x] Reject unknown JSON fields, multiple JSON values, and bodies over the
  configured limit.
- [x] Add test builders for authenticated principals and error assertions; do
  not couple these helpers to tenant schema strings.

**Acceptance:** unit tests cover malformed/oversized JSON, cancellation, encoder
failure, request-ID propagation, and internal-error redaction.

## Merge order

M01-D01 → M01-D02; M01-D03 can follow D01; M01-D04 and D05 can proceed after the
command contract is stable. Do not mix package rearrangement or product feature
work into these changes.

## Exit demonstration

Run the M01 row in [`../verification.md`](../verification.md). Record the merged
PR/commit links here before changing status to `complete`.

**Implementation commits:**

- `0f5ea40` — reproducible verification commands and patched Go 1.26.6 baseline
- `a65f1af` — migration command failure propagation
- `c7a3e27` — disposable dependency harness and isolation smoke test
- `c357b40` — CI parity with repository verification targets
- `d914ea5` — typed configuration and dependency injection
- `763bbfe` — explicit MinIO bootstrap and startup verification
- `2d2d8d6` — health endpoints and bounded process lifecycle
- `0057f0a` — shared bounded HTTP and test primitives

**Local verification (2026-08-19):** `make check`, `make test-unit`,
`make test-race`, two successive `make test-integration` runs,
`make test-isolation`, and the pinned Gitleaks scan pass. The tracked worktree
remains unchanged after verification. Remote CI is pending, so M01 remains
`in progress`.
