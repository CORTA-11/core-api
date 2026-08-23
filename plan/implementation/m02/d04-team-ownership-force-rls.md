# M02-D04 — Team ownership and FORCE RLS

| Field | Value |
| --- | --- |
| Status | `complete` |
| Branch | `security/m02-d04-team-rls` |
| PR title | `security(rls): enforce trusted team ownership` |
| Predecessor | M02-D03 merged to refreshed `main` |
| Dependencies | Trusted executor, current tenant fleet, public identity/JWT primitives |
| Merge gate | Empty/M01 upgrade, unit, integration, isolation, race, generation, and `make check` |

## Outcome and security invariants

Every team-owned production path uses a trusted team context and executes under
forced PostgreSQL RLS with least-privileged runtime credentials. Teams/tasks use
public UUIDs externally; numeric IDs and schemas remain internal. Membership must
be current both when resolving the context and when each policy evaluates.

The runtime role cannot bypass or disable RLS, create schemas, assume owner
privileges, or mutate migration ledgers. An application predicate bug cannot
turn into a cross-team read/write.

## Current repository state and deficiencies

Tenant migrations define numeric teams and nullable `tasks.team_id`; there is no
tenant team-membership table or RLS. Routes/services accept internal team/task
IDs and schema strings, current responses expose numeric IDs/schema names, and
team/task/file paths are not uniformly JWT-protected. Runtime, schema creation,
and migrations currently share `DATABASE_URL` privileges.

## Scope

In scope:

- Add public UUIDs for teams and tasks while retaining internal BIGINT keys.
- Add unique team memberships linked to public user UUIDs.
- Store only closed roles: `team_admin`, `research_lead`, `researcher`,
  `contributor`, `viewer`; permission mapping remains deferred to M03.
- Backfill safe task ownership. Move ambiguous nullable tasks to a dedicated,
  inaccessible quarantine team; never guess ownership. Then set
  `tasks.team_id NOT NULL` and add access-path indexes.
- `ENABLE` and `FORCE ROW LEVEL SECURITY` on teams, memberships, tasks, and every
  other team-owned table, with policies requiring transaction-local user/team
  settings and a current membership.
- Separate owner, migrator, provisioner, and runtime roles/grants.
- Deny runtime schema creation, RLS bypass/disable, ledger mutation, and cross-team access.
- Make API runtime use only `DATABASE_URL`; operational commands use separate
  migration/provisioning URLs.
- Complete trusted team resolution using public identity and current membership.
- Enforce JWT on current team/task/file routes.
- Retain `X-Org-ID` temporarily only as a public organization-ID selector until M03.
- Migrate team, task, and file services to opaque scopes/executor callbacks.
- Remove schema names/numeric IDs from responses, then delete service schema
  derivation and direct search-path code after the caller inventory is empty.

Deferred: granular role-to-permission mapping, OIDC/session redesign, final
organization selector contract, and unrelated domain tables introduced after M02.

## Interfaces, persistence, commands, and compatibility

Public handlers accept/return UUIDs for organizations, teams, and tasks. Services
receive trusted opaque scopes, not schema strings or BIGINT IDs, and run generated
queries only inside executor callbacks. `X-Org-ID` retains its name temporarily
but its value is a public UUID selector and grants no authority.

Public and tenant migrations add identities, memberships, constraints, indexes,
quarantine handling, policies, and grants. Configuration distinguishes:

```text
DATABASE_URL                 runtime API only
MIGRATION_DATABASE_URL       public/tenant migration commands
PROVISIONING_DATABASE_URL    schema/role provisioning commands
```

Removing schema/numeric fields is an intentional security compatibility change;
document affected endpoints and replacement UUID fields in the PR.

## Test-first matrix

| Initial failing test | Expected red result | Passing criterion |
| --- | --- | --- |
| public-ID migration test | Teams/tasks expose only BIGINT identity | Stable unique UUIDs exist and external models omit BIGINT IDs |
| membership/role constraint test | Duplicate/open role values are accepted | User/team uniqueness and closed role check reject invalid rows |
| ownership backfill test | Nullable/ambiguous tasks receive guessed team or remain null | Proven rows map safely; ambiguous rows enter inaccessible quarantine; NOT NULL holds |
| catalog RLS test | Tables lack ENABLE/FORCE/policies/indexes | Catalog proves required flags, policies, constraints, and indexes |
| unsafe generated query test | Missing team predicate returns another team's rows | RLS filters/denies it under runtime credentials |
| role privilege test | Runtime can create schema/change RLS/update ledger | Every privileged operation is denied |
| stale membership test | Resolved context survives membership revocation | Policy rechecks current membership and denies the operation |
| JWT/selector route test | Anonymous or forged selector reaches tenant path | JWT and resolver deny; `X-Org-ID` never influences schema directly |
| service cutover test | Team/task/file service still accepts schema or raw IDs | Only opaque scope/executor callback path compiles and passes |
| response contract test | Schema names/numeric IDs appear in JSON | Only public IDs and allowed domain fields are returned |

## Ordered implementation

1. Record the D03 handoff and bootstrap distinct database roles before installing
   policies that refer to those roles.
2. Add public IDs, membership/role constraints, safe quarantine backfill, NOT NULL, and indexes.
3. Add failing unsafe-query/privilege tests; implement ENABLE/FORCE policies and least-privilege grants.
4. Complete team resolver tests against current membership and public IDs.
5. Add route authentication/contract tests and enforce JWT plus public-ID selection.
6. Cut team, then task, then file services over to trusted executor callbacks.
7. Prove the direct-schema caller inventory is empty; remove schema derivation/search-path code and internal IDs from DTOs.
8. Execute rollout rehearsal and record compatibility/security evidence.

## Atomic green commits

1. `docs(plan): hand off m02-d03 to m02-d04`
2. `security(db): separate tenant database roles`
3. `feat(teams): add public identity and team membership`
4. `security(rls): force team ownership policies`
5. `refactor(tenancy): resolve trusted team contexts`
6. `refactor(teams): use the tenant executor`
7. `refactor(tasks): use the tenant executor`
8. `refactor(files): consume trusted team scope`
9. `refactor(db): remove legacy schema selection`
10. `docs(plan): link m02-d04 implementation`

## Verification and acceptance

Run:

```bash
make generate-check
make test-unit
make test-integration
make test-isolation
make test-race
make check
```

- [x] Empty and M01-upgrade paths preserve data and quarantine ambiguity safely.
- [x] All team-owned tables have correct ownership, grants, ENABLE/FORCE RLS, and policies.
- [x] Runtime credentials fail privileged and cross-team operations.
- [x] Revocation after resolution is enforced by the database.
- [x] Team/task/file routes require JWT and resolve only public IDs.
- [x] Responses expose neither schemas nor numeric database IDs.
- [x] No production caller derives schemas or sets search paths directly.
- [x] API starts with only runtime `DATABASE_URL` credentials.

## Migration, rollout, rollback, and operations

Required rollout order:

1. Bootstrap owner, migrator, provisioner, and runtime roles/grants.
2. Apply the public migration with migration credentials.
3. Apply every tenant migration with migration/provisioning credentials.
4. Verify fleet currency, quarantine counts, catalog policy/grant assertions, and runtime denials.
5. Deploy the API using only the runtime role.

Do not deploy runtime-role code before every active tenant is migrated. A code
rollback may use the previous API only while compatible columns remain; never
rollback by disabling RLS or restoring broad runtime privileges. Database down
migrations must preserve quarantined data and refuse unsafe loss. Alert on
provisioning/migration failures, denied operations by class, and unexpected
quarantine counts without high-cardinality tenant labels.

## Handoff to D05

Provide runtime fixture credentials, privileged setup credentials, the full
team-owned table/policy/index inventory, representative production query paths,
the deliberately predicate-free bounded query, and evidence that all services
use opaque scopes. D05 must test this deployed role topology without weakening it.

### Role, RLS, grant, and index inventory

- `synodus_owner` is non-login and owns `public`, canonical tenant schemas,
  application tables/sequences, both migration ledgers, and tenancy security
  functions. `synodus_migrator` and `synodus_provisioner` are login roles with
  SET-only owner membership and connection-time owner assumption.
  `synodus_runtime` is a login role with no superuser, create-role,
  create-database, replication, or `BYPASSRLS` attribute and cannot assume owner.
- Runtime public grants are explicit CRUD on `orgs`, `users`, and `org_user`,
  plus only the corresponding sequence usage. Both migration ledgers are denied.
  Tenant grants are SELECT on `teams`/`team_members`, task CRUD and task-sequence
  usage, and EXECUTE on the three reviewed security functions.
- `teams`, `team_members`, and `tasks` have both ENABLE and FORCE RLS. Each has an
  owner-maintenance policy. Runtime can list non-quarantine teams and memberships
  only through current membership; task read/write additionally requires the
  transaction-local team setting. Unsupported direct team/membership mutation
  has no runtime policy. `create_team_with_creator` is the sole atomic creation
  path and inserts the authenticated creator as `team_admin`.
- Identity/ownership indexes are the unique team/task UUID constraints,
  `teams_single_quarantine_idx`, `team_members` primary key and
  `team_members_user_team_idx`, and `tasks_team_created_id_idx`. The closed
  stored-role set is `team_admin`, `research_lead`, `researcher`, `contributor`,
  and `viewer`; D04 intentionally gives each current member equal task access.

### Compatibility record

The temporary unversioned `/teams`, `/{team_uuid}/tasks`,
`/{team_uuid}/tasks/{task_uuid}`, and `/{team_uuid}/files/...` routes remain.
Every route now requires the existing bearer JWT. `X-Org-ID` remains only as a
public organization UUID selector. Missing/invalid authentication returns 401,
malformed UUID selectors return 400, and unavailable, unknown, or nonmember
trusted contexts share the same 404 response. Team/task JSON uses `public_id` and
contains no numeric database ID, schema, or task `team_id`. Existing MinIO keys
remain stable through the tenancy-owned storage-scope helper.

## Implementation record

**Merged PR:** _pending_

**Merge commit:** _pending_

**Branch commits:**

- D03 handoff and corrected D04 ordering: `b080343`
- Database-role separation and ledger ownership: `0e2190a`
- Team/task public identity, membership, backfill, and quarantine: `861912f`
- Forced RLS, runtime policies, grants, and unsafe-query proof: `87f04b6`
- UUID/current-membership team context: `6520716`
- Team service/handler executor cutover: `6967fb1`
- Task service/handler executor cutover: `c797d60`
- File route/service trusted-scope cutover: `36a4c90`
- Legacy schema-selection removal and repository guard: `e612143`

**Test-first evidence:** role/config tests initially failed because migrate/seed
still used `DATABASE_URL` and provisioning fell back to it. The disposable
rollout then caught a missing owner transfer for `public.schema_migrations`;
`TestDatabaseRolesAreSeparatedAndRuntimeCannotMutateLedger` reproduced the
failure (`integration` owner instead of `synodus_owner`) before the migration
fix passed. The v2 identity upgrade initially failed because tenant v3 and its
safe down behavior did not exist. RLS catalog/runtime tests initially reported
missing ENABLE/FORCE flags and functions. Resolver, service, handler, and storage
focused tests then drove UUID/current-membership contexts, opaque executor
callbacks, JWT-only routes, public DTOs, and preserved object keys. The final
repository guard includes a forged service-local `SET LOCAL search_path` fixture
and proves only the explicit executor/reconciler/test-fixture allowlist passes.

**Migration and isolation evidence:** real PostgreSQL 18 tests cover fresh public
and tenant databases, current-v2 tenant upgrades, unique UUIDs, viewer backfill,
one deliberate null-owned task entering exactly one inaccessible quarantine
team, non-null/index/catalog contracts, role attributes and SET-only membership,
ledger ownership/denial, missing/invalid settings, predicate-free task reads,
cross-team filtering, post-resolution membership revocation, atomic creator
membership, and runtime denial of schema creation, owner assumption, RLS disable,
membership mutation, and ledger writes. Fresh tenants produce no unexpected
quarantine rows; the upgrade fixture's single ambiguous row produces its one
expected quarantine row without changing valid ownership.

**Rollout rehearsal:** on 2026-08-23 a disposable PostgreSQL 18/Redis/MinIO stack
bootstrapped the four roles, applied public migration version 5, assigned
disposable role login secrets, and verified the migrator reported version 5
clean while the provisioner inspected the full (empty) tenant registry. MinIO
was bootstrapped, then the API reached `/health/ready` with HTTP 204 using only a
`synodus_runtime` `DATABASE_URL`; no migration or provisioning URL was supplied
to the API. The runtime role could not take privileged actions. The stack,
network, and all disposable volumes were removed after rehearsal. Upgrade and
tenant-reconciliation paths are additionally exercised by the integration lane.

**Green verification:** `make generate-check`, `make test-unit`,
`make test-integration`, `make test-isolation`, `make test-race`, `make check`,
and `git diff --check` passed on 2026-08-23. `make check` included build,
format/module/generation/migration/query checks, vet, golangci-lint, gopls
diagnostics, govulncheck (zero called vulnerabilities), and gosec. M02 remains
active; D05 owns the larger adversarial matrix, two-connection 4,000-operation
stress proof, final merged links, and milestone completion.
