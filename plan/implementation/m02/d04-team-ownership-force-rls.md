# M02-D04 — Team ownership and FORCE RLS

| Field | Value |
| --- | --- |
| Status | `planned` |
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

1. Add failing migration/backfill/catalog tests and bootstrap distinct database roles.
2. Add public IDs, membership/role constraints, safe quarantine backfill, NOT NULL, and indexes.
3. Add failing unsafe-query/privilege tests; implement ENABLE/FORCE policies and least-privilege grants.
4. Complete team resolver tests against current membership and public IDs.
5. Add route authentication/contract tests and enforce JWT plus public-ID selection.
6. Cut team, then task, then file services over to trusted executor callbacks.
7. Prove the direct-schema caller inventory is empty; remove schema derivation/search-path code and internal IDs from DTOs.
8. Execute rollout rehearsal and record compatibility/security evidence.

## Atomic green commits

1. `feat(teams): add public identity and team membership`
2. `security(rls): force team ownership policies`
3. `security(db): separate tenant database roles`
4. `refactor(tenancy): resolve trusted team contexts`
5. `refactor(teams): use the tenant executor`
6. `refactor(tasks): use the tenant executor`
7. `refactor(files): consume trusted team scope`
8. `docs(plan): link m02-d04 implementation`

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

- [ ] Empty and M01-upgrade paths preserve data and quarantine ambiguity safely.
- [ ] All team-owned tables have correct ownership, grants, ENABLE/FORCE RLS, and policies.
- [ ] Runtime credentials fail privileged and cross-team operations.
- [ ] Revocation after resolution is enforced by the database.
- [ ] Team/task/file routes require JWT and resolve only public IDs.
- [ ] Responses expose neither schemas nor numeric database IDs.
- [ ] No production caller derives schemas or sets search paths directly.
- [ ] API starts with only runtime `DATABASE_URL` credentials.

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

## Implementation record

**Merged PR:** _pending_

**Merge commit:** _pending_

