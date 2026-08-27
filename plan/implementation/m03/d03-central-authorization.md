# M03-D03 — Central authorization

| Field | Value |
| --- | --- |
| Status | `implemented; PR pending` |
| Branch | `security/m03-d03-authorization` |
| PR title | `security(authz): enforce role permissions` |
| Predecessor | M03-D02 merged |
| Dependencies | M02 resolver/executor and D02 authenticated principal |
| Merge gate | `make check`, unit, integration, isolation, race, and generate checks |

## Outcome and security invariants

One typed, default-deny authorization service maps current database roles to
named permissions and wraps M02's trusted tenant resolver/executor. Organization
administration and team-content access remain separate capabilities.

- Roles and permissions are closed types. Unknown strings never inherit a
  default or partial mapping.
- Authentication proves only a user public ID/session. Every organization/team
  membership, role, lifecycle, and resource scope comes from PostgreSQL.
- Protected mutations re-read membership and role inside the same transaction
  that changes state; a pre-handler decision alone is insufficient.
- Missing context, removed membership, ownerless administrative action, unknown
  lifecycle, guessed UUID, wrong organization/team, and cross-team object all
  deny without leaking existence.
- RLS remains defense in depth; authorization cannot weaken M02 executor or
  runtime-role requirements.

## Current state and deficiencies

`public.org_user` permits duplicates and has no role, timestamps, or key. Tenant
memberships have the closed M02 role names but no permission map. Existing JWT
middleware authenticates only selected routes; resolver membership proves
visibility but not operation-level authority. Organization routes are public,
organization creation does not establish an owner, and no safe owner-bootstrap
operator path exists.

## Scope

### Organization membership migration

- Lock `public.org_user` for the migration; deduplicate identical `(org_id,
  user_id)` pairs because existing rows carry no conflicting role information.
- Add `role` constrained to `owner`, `administrator`, or `member`, plus bounded
  timestamps and a primary/unique key on `(org_id,user_id)`.
- Backfill every legacy membership as least-privileged `member`. Never infer an
  owner from row order, email, creation time, team role, or organization creator
  history.
- Add `cmd/admin org owner assign --org <public UUID> --user <public UUID>`.
  It is idempotent, validates active records, runs transactionally, and emits an
  audit-ready result without internal IDs. An operator must assign at least one
  owner to every active legacy organization before D06.
- New organization creation atomically creates the organization membership with
  the authenticated creator as `owner`; tenant provisioning continues through
  the existing lifecycle and does not broaden that role into team membership.

Ownerless legacy organizations remain readable to current organization members.
Every organization/team administrative mutation fails closed until an owner is
assigned; normal team content remains governed only by existing team membership.

### Typed permissions and role mapping

Use closed constants (names shown as wire/audit strings):

```text
org.read org.update org.delete org.restore
org.members.read org.members.manage org.owners.manage
team.create team.read team.update team.delete team.members.read team.members.manage
task.read task.create task.update task.move task.delete
file.read file.upload file.delete
audit.read realtime.connect
```

Organization mapping:

| Role | Permissions |
| --- | --- |
| `owner` | all `org.*`, including owner management and lifecycle; `team.create` |
| `administrator` | `org.read`, `org.update`, member read/manage excluding owners; `team.create` |
| `member` | `org.read` only |

An administrator cannot add, remove, promote, demote, or otherwise mutate an
owner and cannot delete/restore the organization. The last owner cannot be
removed or demoted. Owner changes lock the organization's membership set and
preserve at least one owner under concurrency.

Team mapping:

| Role | Permissions |
| --- | --- |
| `team_admin` | team/member management; every task, file, audit, and realtime permission |
| `research_lead` | `team.read`; all task/file permissions; `audit.read`; `realtime.connect` |
| `researcher` | `team.read`; task read/create/update/move; file read/upload; realtime |
| `contributor` | `team.read`; task read/create/update; file read/upload; realtime |
| `viewer` | team/task/file read and realtime only |

Organization owner/administrator status grants `team.create` but no implicit
`team.read`, task, file, audit, or realtime permission.

## Interfaces and denial contract

`internal/authorization` defines `Permission`, `OrganizationRole`, `TeamRole`,
the immutable mapping, and an `Authorizer`. Read paths authorize current
membership before invoking an M02 executor callback. Mutation paths begin the
executor transaction, resolve route public IDs, lock/re-read relevant membership
and lifecycle, authorize, mutate, and commit in that order.

Return internal `ErrUnauthenticated`, `ErrOperationDenied`, and
`ErrResourceNotFound` classes without database detail. D04 maps absent/session
failure to `401`, known operation-level denial where no protected ID is being
probed to `403`, and missing-or-unauthorized protected IDs to one `404` problem.
Handlers never accept a requested role as proof or select schema/team context.

## Test-first matrix

| Initial failing test/check | Expected red result | Passing criterion |
| --- | --- | --- |
| org migration upgrade | duplicates survive or an owner is guessed | duplicates collapse deterministically; all roles are `member`; no guessed owner |
| role × permission generated table | implicit/unknown permissions pass | every declared pair matches tables and all unknowns deny |
| owner concurrency test | two transactions remove the last owner | locking/constraint path preserves one owner or rejects both unsafe changes |
| ownerless legacy test | admin mutation succeeds or reads disappear | current members can read; all administrative mutations deny until CLI assignment |
| creator ownership test | org and owner membership can split | creation/owner insert is atomic; failure leaves neither partial state |
| separation test | org admin reads team content without membership | team-content permission denies until an explicit team role exists |
| mutation revocation race | handler check survives membership removal | transaction re-read denies and protected state remains unchanged |
| resolver/RLS matrix | wrong team/org/guessed ID leaks or succeeds | uniform deny/`not found`; same-scope allowed operation succeeds |
| lifecycle/unknown test | unknown state/role/permission falls through | every unrecognized or missing value denies |
| operator command test | internal ID/ambiguous target accepted | public IDs only, idempotent assignment, safe concurrent behavior |

Migration, owner/membership concurrency, mutation races, lifecycle, resolver, and
RLS tests use real PostgreSQL/runtime roles. Pure mapping completeness uses fast
table-generated unit tests.

## Ordered implementation

1. Add failing fresh/upgrade/duplicate/ownerless migration tests; add the public
   membership migration and regenerate queries.
2. Add exhaustive role-permission tests, then implement closed types and immutable
   maps with unknown-value rejection.
3. Add owner assignment and last-owner concurrency tests; implement the operator
   command and transaction primitives.
4. Add organization-creation rollback tests; atomically establish creator owner
   membership without granting a team role.
5. Add read and mutation authorization matrices around the M02 resolver/executor,
   including transaction-time revocation and cross-team probes.
6. Integrate the dark v1 service/handler paths, run isolation/race regressions,
   and record migration plus red/green evidence.

## Atomic green commits

1. `security(authz): add closed organization roles`
2. `security(authz): define role permission mappings`
3. `feat(admin): assign organization owners safely`
4. `security(authz): authorize trusted tenant execution`
5. `test(authz): prove transactional permission boundaries`
6. `docs(plan): link m03-d03 implementation`

## Verification and acceptance

```bash
make generate-check
make test-unit
make test-race
make test-integration
make test-isolation
make check
git diff --check
```

- [x] Upgrade deduplicates safely, grants least privilege, and guesses no owner.
- [x] Every role × permission pair and every unknown value is tested.
- [x] Ownerless/read-only and last-owner invariants hold under concurrency.
- [x] New organization creation establishes exactly one creator-owner atomically.
- [x] Organization administration never implies team-content access.
- [x] Mutations re-read current permission in their state-changing transaction.
- [x] Wrong-scope and guessed identifiers default deny through real RLS execution.
- [x] Prepared branch records red and green evidence; PR evidence remains pending.

## Rollout, rollback, and operations

Rehearse fresh and M02 upgrades. Applying the role migration intentionally leaves
legacy organizations ownerless. Inventory all active organizations and assign
owners explicitly before D06; the command output becomes deployment evidence.
Alert only on aggregate ownerless counts, never membership PII.

Rollback application code can leave additive roles in place. Do not drop role
constraints or recreate duplicate membership rows. If authorization code fails,
hold/roll forward the application; never substitute JWT claims, handler-only
checks, or organization administration for team permission.

## Handoff to D04

Provide permission constants/maps, principal and decision-error contracts,
transactional authorization callbacks, owner-bootstrap inventory evidence, and
the exhaustive role/cross-scope tests. D04 fixes their HTTP representation and
documents every protected operation in OpenAPI.

## Implementation record

**Pull request:** _pending_

**Merge commit:** _pending_

**Implementation commits:** `28c61f0` closed organization-role migration;
`515f15b` permission vocabulary and exhaustive mappings; `38744b3` safe owner
assignment and owner-set locking; `565a139` trusted transactional authorization
and creator ownership; `695395c` PostgreSQL adversarial proofs; this documentation
commit.

**Observed red evidence:**

- The first `make generate` run failed on the expected ambiguous owner-assignment
  query parameter; qualifying the user source made the generated-query boundary
  explicit.
- The first `make test-integration` run failed on generated canonical-email test
  fixtures and showed that the new latest down migration weakened the existing
  forward-only rollback contract. The fixtures now write only the source email,
  and migration `000009` refuses destructive rollback.

**Green evidence:** focused authorization, tenancy, admin, and service unit tests;
`make generate-check`; `git diff --check`; Docker-backed `make test-integration`;
and Docker-backed `make test-isolation` passed on the prepared branch. The
database lanes cover M02-to-D03 duplicate upgrade, no guessed owner, atomic
creator ownership, concurrent owner removal, transaction-time organization
revocation, lifecycle changes, guessed team UUIDs, and organization/team role
separation. Final repository-wide gate results are recorded in the PR.
