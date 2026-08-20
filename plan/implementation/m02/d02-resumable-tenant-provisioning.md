# M02-D02 — Resumable tenant provisioning

| Field | Value |
| --- | --- |
| Status | `planned` |
| Branch | `feat/m02-d02-tenant-provisioning` |
| PR title | `feat(tenancy): make tenant provisioning resumable` |
| Predecessor | M02-D01 merged to refreshed `main` |
| Dependencies | `publicdb`/`tenantdb` split; disposable PostgreSQL harness |
| Merge gate | Unit/integration tests, empty and M01-upgrade migrations, `make check`, `make generate-check` |

## Outcome and security invariants

Organization creation records durable provisioning intent; a retryable worker or
one-shot command allocates the canonical schema and advances it to the current
tenant version. Only an `active` organization at the reconciled current version
is usable. Partial failure is sanitized, visible to operators, and safe to retry.

Schema names derive solely from a server-generated organization UUID already
stored in `public.orgs`. Concurrent provisioning for one organization is
serialized. Migration application and its ledger update are atomic per version.

## Current repository state and deficiencies

Organization creation currently derives a schema in `internal/service/org_impl.go`,
creates it in the request transaction, and invokes migrations directly. The
registry has no lifecycle/version/error state, tenant schemas have no per-schema
ledger, and commands cannot report or repair a fleet. M01 schemas contain tenant
migrations 000001–000002 but no adoption record.

## Scope

In scope:

- Add organization state (`provisioning`, `active`, `failed`, `deleting`),
  canonical schema, reconciled tenant version, sanitized last error, and timestamps.
- Define validated transitions; failure never leaves an organization active.
- Allocate the schema from the stored server-generated organization UUID.
- Create a per-tenant ledger containing version, checksum, and application time.
- Adopt existing M01 schemas/migrations only after catalog validation and checksum reconciliation.
- Apply each migration transactionally under a per-organization advisory lock.
- Reconcile filesystem, registry, and ledger versions; reject checksum/version divergence.
- Make success, already-current execution, failure, and retry idempotent.
- Provide single/all pending provision commands and public/tenant apply, status,
  and retry commands.
- Bound fleet concurrency to default 4, configurable maximum 16; print one
  sanitized result per tenant and exit non-zero if any tenant fails.
- Change organization creation to enqueue provisioning rather than doing schema work inline.

Deferred: tenant request execution, team RLS, background queue infrastructure,
permission mapping, and deletion/purge implementation beyond the lifecycle contract.

## Interfaces, persistence, commands, and compatibility

Add a provisioner interface keyed by organization public UUID—not schema name—and
a migration source abstraction suitable for failure injection. Public migrations
add lifecycle fields/constraints. Tenant bootstrap creates/adopts the ledger
before applying later tenant migrations.

The command contract must support equivalent operations such as:

```text
provision one --organization <public-uuid>
provision pending --concurrency <1..16>
migrate public apply|status
migrate tenant apply|status|retry --organization <public-uuid>
migrate tenant apply|status|retry --all --concurrency <1..16>
```

Exact CLI spelling may follow existing `cmd/migrate`, but raw schema names are
never arguments. API organization creation may become asynchronous: it returns
the public organization/lifecycle state and does not expose a schema name.

## Test-first matrix

| Initial failing integration test | Expected red result | Passing criterion |
| --- | --- | --- |
| empty database bootstrap | Lifecycle columns/ledger absent | Public and tenant migrations create a provisionable current tenant |
| M01 upgrade/adoption | Existing schema has no ledger and cannot reconcile | Valid M01 objects are checksummed/adopted without data loss |
| successful provision | Request path performs ad hoc schema migration | Provisioner reaches `active` and current version idempotently |
| injected migration failure | Partial schema has no durable safe state | Transaction rolls back, org becomes `failed`, sanitized detail is recorded |
| retry after failure | Retry duplicates objects or remains failed | Retry resumes at last committed version and reaches `active` |
| concurrent provision calls | Same org races DDL/migrations | Advisory lock serializes; both callers observe one consistent result |
| already-current tenant | Reapplication errors or rewrites ledger | No-op success leaves ledger/version unchanged |
| fleet partial failure | Fleet exits success or hides per-tenant outcome | Other tenants finish; all results print; process exits non-zero |
| concurrency bounds | Zero or >16 starts invalid fan-out | Default is 4; values outside 1–16 are rejected or safely normalized by contract |

## Ordered implementation

1. Write empty/M01-upgrade migration tests and add lifecycle/ledger migrations.
2. Write lifecycle transition unit tests, then implement closed state transitions and sanitization.
3. Write canonical allocation/reconciliation tests, then implement stored-UUID resolution and checksums.
4. Write failure, retry, concurrency, and already-current tests; implement advisory-locked transactional application.
5. Write CLI/fleet tests; implement single/fleet apply, status, retry, bounds, results, and exit codes.
6. Write organization-handler/service regression tests, then replace inline provisioning with durable enqueue state.
7. Verify all active organizations are current and record handoff evidence.

## Atomic green commits

1. `feat(orgs): record tenant provisioning lifecycle`
2. `feat(tenancy): add resumable tenant provisioning`
3. `feat(migrations): add bounded tenant fleet commands`
4. `refactor(orgs): enqueue organizations for provisioning`
5. `docs(plan): link m02-d02 implementation`

## Verification and acceptance

Run:

```bash
make test-unit
make test-integration
make generate-check
make check
```

Also execute documented empty-database and M01-snapshot upgrade paths twice.

- [ ] State transitions and database constraints reject invalid lifecycle states.
- [ ] Existing M01 tenants are adopted without replay or data loss.
- [ ] Checksums, locks, rollback, idempotency, and retry are proven in PostgreSQL.
- [ ] CLI accepts only public organization IDs and bounds concurrency to 1–16.
- [ ] Partial fleet failure produces per-tenant results and non-zero exit.
- [ ] Errors and logs contain no credentials, raw SQL, or sensitive internals.
- [ ] Every active organization reports the current tenant version.

## Migration, rollout, rollback, and operations

Apply public lifecycle migrations first. Deploy/use migration credentials to
adopt and advance tenant schemas; do not mark them active until reconciliation
succeeds. Operators can retry failed organizations after correcting the cause.
Down migration must refuse destructive ledger/lifecycle removal while tenants
depend on it; operational rollback is to stop provisioning, deploy the prior API,
and retain additive state for forward repair. Back up the public registry before
fleet adoption. Monitor counts by lifecycle/version with bounded-cardinality labels.

## Handoff to D03

D03 is blocked until a fleet status run proves every active organization is at
the current tenant version. Hand off canonical schema derivation/validation,
registry lookup APIs, lifecycle semantics, ledger contract, runtime/migration
credentials available to tests, and commands/results used to establish currency.

## Implementation record

**Merged PR:** _pending_

**Merge commit:** _pending_
