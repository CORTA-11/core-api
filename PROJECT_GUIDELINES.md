# Synodus — Engineering and Product Development Guideline

This document defines product direction and system architecture. See
[`STYLE.md`](STYLE.md) for implementation standards,
[`CONTRIBUTION.md`](CONTRIBUTION.md) for the contribution workflow, and
[`AGENTS.md`](AGENTS.md) for the concise repository operations index.

**Status:** Project guideline
**Scope:** Architecture, implementation, security, operations, testing, deployment and product engineering
**Target quality:** Production-oriented engineering above normal university-project and small-company standards
**Primary constraint:** Achievable by a small, highly motivated engineering team

---

## 1. Purpose

Synodus is a privacy-first, self-hostable research collaboration and research-operations platform.

It is intended for:

* research laboratories;
* university research groups;
* R&D teams;
* privacy-sensitive technical organizations;
* collaborative scientific and engineering projects.

Synodus combines collaboration features with systems concerns that are usually absent from university projects:

* strong multi-tenancy;
* team-level database isolation;
* privacy-preserving file storage;
* durable asynchronous processing;
* concurrency control;
* realtime collaboration;
* research artifact provenance;
* auditable operations;
* controlled AI assistance;
* measurable reliability;
* observability;
* security verification;
* backup and recovery;
* reproducible self-hosted deployment.

The primary engineering objective is not maximum feature count.

The objective is:

> Build a system whose important invariants remain correct under concurrency, retries, process failure, malicious requests, tenant transitions and infrastructure degradation.

---

## 2. Requirements Language

Within this document:

* **MUST** — project requirement;
* **MUST NOT** — prohibited;
* **SHOULD** — expected unless there is a documented reason not to;
* **MAY** — optional;
* **STRETCH** — worthwhile only after the critical system is complete.

These terms describe Synodus project policy.

---

## 3. Fixed Architectural Decisions

The following decisions are already settled and MUST NOT be redesigned without an explicit architecture change.

### 3.1 Organization isolation

Every organization receives its own PostgreSQL schema.

```text
PostgreSQL database

public
org_a8f...
org_d31...
org_f19...
...
```

Organization-owned data does not share ordinary application tables with other organizations.

---

### 3.2 Team isolation

Teams exist inside an organization schema.

Team-owned rows contain:

```text
team_id BIGINT NOT NULL
```

Team-level isolation is enforced using PostgreSQL Row-Level Security where applicable.

```text
Organization
    ↓
PostgreSQL schema

Team
    ↓
team_id + RLS
```

---

### 3.3 Internal and external identity

Internal relational identifiers:

```text
BIGINT
```

External identifiers:

```text
UUID
```

Clients MUST NOT use internal numeric database IDs.

---

### 3.4 Current core stack

Backend:

```text
Go
chi
pgx/v5
sqlc
PostgreSQL
```

Infrastructure:

```text
Redis
MinIO
```

Frontend:

```text
React
TypeScript
Tiptap
Yjs
```

AI:

```text
separate Python service
```

These remain the foundation unless a later ADR explicitly changes them.

---

## 4. Standards Baseline

Security verification SHOULD target **OWASP ASVS 5.0.0**, currently the stable ASVS release, using Level 2 as the general baseline with stronger selected requirements for authentication, cryptography, administration and tenant isolation.

Any future OAuth/OIDC integration MUST follow current OAuth security guidance rather than old OAuth examples; RFC 9700 is the current OAuth 2.0 Security Best Current Practice. OAuth/OIDC is not a current runtime dependency.

PostgreSQL RLS MUST be designed around the actual PostgreSQL semantics: superusers and `BYPASSRLS` roles bypass policies, table owners normally do so as well, while `FORCE ROW LEVEL SECURITY` can subject the owner to policies.

OpenTelemetry is the standard observability abstraction for Synodus. It provides a common model for traces, metrics and logs and supports routing them through the OpenTelemetry Collector.

HTTP errors SHOULD use RFC 9457 Problem Details rather than a custom ad-hoc error envelope. RFC 9457 supersedes RFC 7807.

Synodus SHOULD maintain an OpenAPI contract. OpenAPI defines a language-independent interface description for HTTP APIs. The project should standardize on a tested OpenAPI 3.1.x toolchain even though newer specification revisions may exist.

---

## 5. Product Position

Synodus should not become merely:

```text
Slack
+
Trello
+
Google Docs
+
file upload
```

The distinguishing engineering/product capabilities should be:

1. secure research collaboration;
2. organization and team isolation;
3. encrypted research data storage;
4. experiment and artifact provenance;
5. reproducible research metadata;
6. realtime coordination;
7. audited administrative activity;
8. privacy-aware AI;
9. self-hosted operation.

---

## 6. High-Level Domain Model

```text
Organization
│
├── Organization Members
│
├── Teams
│   │
│   ├── Team Members
│   ├── Projects
│   ├── Tasks
│   ├── Channels
│   │   └── Messages
│   ├── Documents
│   ├── Files
│   ├── Datasets
│   ├── Experiments
│   ├── Research Artifacts
│   ├── Resources
│   │   └── Bookings
│   ├── Notifications
│   └── Audit Events
│
└── Organization Administration
```

A team is the primary collaboration/security unit within an organization.

A project MAY group research activities inside a team but is not itself a tenancy boundary.

---

## 7. System Architecture

```text
                            Browser
                               │
                          HTTPS / WSS
                               │
                      ┌────────▼────────┐
                      │ Caddy / Reverse │
                      │ Proxy           │
                      └────────┬────────┘
                               │
                     ┌─────────▼───────────┐
                     │     Go Backend      │
                     │                     │
                     │ API / BFF           │
                     │ authorization       │
                     │ tenancy             │
                     │ domain services     │
                     └──┬─────┬─────┬────┘
                        │     │     │
              ┌─────────┘     │     └─────────┐
              ▼               ▼               ▼
         PostgreSQL          Redis           MinIO
         durable state     ephemeral       object storage
              │            coordination
              │
              ▼
        Durable workers
              │
       ┌──────┴──────┐
       ▼             ▼
   realtime       AI jobs
       │             │
       ▼             ▼
 Centrifugo     Python AI Service
       │
       ▼
    Clients


                 Observability plane

Go / Python / infrastructure
             │
             ▼
      OpenTelemetry
             │
       OTel Collector
        /     |      \
       ▼      ▼       ▼
 Prometheus  Tempo    Loki
        \      |      /
             Grafana
```

---

## 8. Deployment Units

The system SHOULD contain only a small number of deployable application components.

Recommended:

```text
synodus-web
synodus-api
synodus-worker
synodus-provisioner
synodus-ai
synodus-migrate
synodus-admin
```

Supporting infrastructure:

```text
postgres
redis
minio
centrifugo
caddy

otel-collector
prometheus
grafana
tempo
loki
```

Do not split individual business domains into microservices.

---

## 9. Architectural Style

The Go application MUST remain a modular monolith.

For example:

```text
internal/
├── identity/
├── organization/
├── team/
├── authorization/
├── task/
├── chat/
├── document/
├── file/
├── dataset/
├── experiment/
├── artifact/
├── resource/
├── booking/
├── notification/
├── audit/
├── tenancy/
├── realtime/
├── ai/
└── platform/
```

Modules represent actual domain responsibilities.

Do not create packages merely to satisfy an architectural pattern.

---

## 10. Dependency Direction

Preferred dependency direction:

```text
HTTP / transport
       ↓
application service
       ↓
domain rules
       ↓
ports / repositories
       ↓
infrastructure
```

Infrastructure MUST NOT become the business model.

For example:

```text
TaskService
   ↓
TaskRepository
```

not:

```text
TaskService
   ↓
Redis
   ↓
SQL
   ↓
MinIO
```

scattered directly throughout business code.

---

## 11. Repository Structure

Target direction:

```text
.
├── cmd/
│   ├── api/
│   │   └── main.go
│   ├── worker/
│   │   └── main.go
│   ├── provisioner/
│   │   └── main.go
│   ├── migrate/
│   │   └── main.go
│   └── admin/
│       └── main.go
│
├── internal/
│   ├── app/
│   ├── identity/
│   ├── organization/
│   ├── team/
│   ├── authorization/
│   ├── task/
│   ├── chat/
│   ├── document/
│   ├── file/
│   ├── dataset/
│   ├── experiment/
│   ├── artifact/
│   ├── resource/
│   ├── booking/
│   ├── notification/
│   ├── audit/
│   ├── tenancy/
│   ├── jobs/
│   ├── realtime/
│   ├── ai/
│   └── platform/
│       ├── postgres/
│       ├── redis/
│       ├── minio/
│       ├── telemetry/
│       └── http/
│
├── db/
│   ├── migrations/
│   │   ├── public/
│   │   └── tenant/
│   ├── queries/
│   │   ├── public/
│   │   └── tenant/
│   └── seeds/
│
├── api/
│   ├── openapi.yaml
│   └── asyncapi.yaml
│
├── ai/
├── web/
│
├── deployments/
│   ├── compose/
│   └── caddy/
│
├── observability/
│   ├── otel/
│   ├── prometheus/
│   └── grafana/
│
├── security/
│   ├── threat-model.md
│   ├── asvs.md
│   └── abuse-cases.md
│
├── docs/
│   ├── adr/
│   ├── architecture/
│   └── runbooks/
│
├── tests/
│   ├── integration/
│   ├── isolation/
│   ├── failure/
│   ├── e2e/
│   └── load/
│
├── compose.yaml
├── Makefile
├── sqlc.yaml
├── go.mod
└── README.md
```

Do not create empty directories merely to match this target.

---

## 12. PostgreSQL Architecture

PostgreSQL has two logical data classes.

### Shared/control data

Lives in:

```text
public
```

Examples:

```text
users
organizations
organization_memberships
sessions
idempotency_records
provisioning state
global audit
```

### Organization data

Lives in:

```text
org_<opaque_id>
```

Example:

```text
org_a82f...
├── teams
├── team_members
├── projects
├── tasks
├── channels
├── messages
├── documents
├── files
├── datasets
├── experiments
├── artifacts
├── resources
├── bookings
├── notifications
├── audit_events
└── schema_migrations
```

---

## 13. Harden the Public Schema

The application MUST explicitly remove unnecessary public DDL privileges.

For example:

```sql
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
```

Application runtime users MUST NOT be able to modify schema structure.

Shared tables SHOULD always be referenced explicitly:

```sql
public.organizations
```

rather than relying on `search_path`.

---

## 14. PostgreSQL Roles

Use separate roles.

Recommended:

```text
synodus_owner
synodus_runtime
synodus_migrator
synodus_provisioner
synodus_readonly
```

### `synodus_owner`

* `NOLOGIN`;
* owns application schemas/tables;
* not used by normal processes.

### `synodus_runtime`

Used by API and ordinary workers.

MUST NOT have:

```text
SUPERUSER
BYPASSRLS
CREATE DATABASE
CREATE ROLE
arbitrary CREATE SCHEMA
```

It SHOULD NOT own tenant tables.

### `synodus_migrator`

Used by deployment migration commands.

May alter application schema.

Credentials are not available to the API.

### `synodus_provisioner`

Used only for creating and initializing organization schemas.

Its privileges MUST be narrower than a PostgreSQL superuser.

---

## 15. Organization Schema Naming

Schema names MUST be generated by trusted backend code.

Never derive SQL identifiers from organization names.

Preferred:

```text
org_<random opaque identifier>
```

Example:

```text
org_7b2f6cbca41e4f95
```

Store the mapping:

```text
public.organizations.schema_name
```

Clients never receive or submit the schema name.

Schema names are implementation details, not authorization credentials.

---

## 16. Organization Lifecycle

Organizations SHOULD have an explicit lifecycle.

```text
PROVISIONING
     │
     ▼
ACTIVE
     │
     ├────► SUSPENDED
     │
     ▼
DELETING
     │
     ▼
DELETED

PROVISIONING
     │
     ▼
PROVISIONING_FAILED
```

This prevents partial infrastructure operations from being represented as healthy organizations.

---

## 17. Organization Provisioning

Provisioning SHOULD be asynchronous.

```text
POST /api/v1/orgs
            │
            ▼
validate
            │
            ▼
BEGIN
 ├── INSERT organization(PROVISIONING)
 ├── INSERT owner membership
 └── enqueue provisioning operation
COMMIT
            │
            ▼
202 Accepted
            │
            ▼
Provisioner
 ├── acquire organization lock
 ├── CREATE SCHEMA
 ├── apply tenant migrations
 ├── initialize mandatory metadata
 └── mark ACTIVE
```

The operation MUST be idempotent.

A provisioner crash must not permanently corrupt organization state.

---

## 18. Provisioning Lock

Only one process may provision a specific organization at a time.

PostgreSQL advisory locks are suitable for this scope.

Conceptually:

```text
organization ID
     ↓
advisory lock
     ↓
provision
```

---

## 19. Tenant Migration Architecture

Two migration streams exist:

```text
db/migrations/public
db/migrations/tenant
```

Public migrations run once.

Tenant migrations run against every organization schema.

---

## 20. Tenant Migration Ledger

Each tenant schema MUST contain migration metadata.

Example:

```text
schema_migrations
-----------------
version
checksum
applied_at
```

Historical migration files MUST become immutable once shared.

Checksums SHOULD detect accidental modification.

---

## 21. New Organization Migrations

When creating a new organization:

```text
CREATE SCHEMA
     ↓
apply tenant migration 1
     ↓
migration 2
     ↓
...
     ↓
current version
```

A new organization MUST start on the current tenant schema version.

---

## 22. Existing Organization Migration Fleet

`cmd/migrate` SHOULD:

```text
migrate public
      ↓
load active organizations
      ↓
for each organization
    acquire migration lock
    inspect tenant version
    apply missing migrations
    record result
      ↓
report failures
```

Bounded concurrency MAY be used.

Do not run hundreds of migrations simultaneously.

---

## 23. Migration Failure

A failed tenant migration MUST identify:

```text
organization public ID
schema
migration version
error
```

Operational tooling must make failed schemas discoverable.

Do not silently continue deployment and pretend the fleet is healthy.

---

## 24. Schema Evolution

Use expand/contract migrations for changes that need compatibility.

Example:

```text
1. add new field
2. deploy code capable of both versions
3. backfill
4. switch reads/writes
5. later remove old field
```

Demonstrating this correctly once is more valuable than implementing elaborate zero-downtime migration infrastructure.

---

## 25. Tenant Request Resolution

Canonical path:

```text
HTTP request
     │
     ▼
application session
     │
     ▼
authenticated user
     │
organization UUID
     ▼
public.organizations
     │
     ├── schema_name
     └── internal organization ID
     │
     ▼
verify organization membership
     │
     ▼
enter organization schema
     │
team UUID
     ▼
resolve team
     │
verify team membership
     │
     ▼
Trusted TenantContext
```

Only trusted code creates `TenantContext`.

---

## 26. Tenant Context

Conceptually:

```go
type TenantContext struct {
	UserID              int64
	UserPublicID        uuid.UUID

	OrganizationID       int64
	OrganizationPublicID uuid.UUID
	SchemaName           string

	TeamID               int64
	TeamPublicID         uuid.UUID
}
```

This object MUST originate from authenticated server-side resolution.

Never construct it from arbitrary handler fields.

---

## 27. Organization and Team Execution Scopes

Use two explicit scopes.

### Organization scope

Used to:

```text
list teams
resolve a team
manage organization-level tenant metadata
```

Sets:

```text
search_path
app.user_id
```

but no team context.

### Team scope

Used for team-owned resources.

Sets:

```text
search_path
app.user_id
app.team_id
```

This makes accidental teamless queries more visible.

---

## 28. Search Path

Tenant operations MUST occur inside transactions.

Use:

```text
BEGIN
SET LOCAL ...
...
COMMIT
```

Never use connection-persistent:

```sql
SET search_path ...
```

on pooled connections.

The transaction-local setting prevents tenant state remaining on a reused connection after transaction completion.

---

## 29. Secure Search Path

Tenant execution SHOULD resolve built-in PostgreSQL objects before tenant-defined objects.

Conceptually:

```text
pg_catalog
tenant schema
```

Do not place arbitrary writable schemas into the path.

Shared application tables should be referenced explicitly as:

```text
public.table_name
```

---

## 30. Dynamic Schema Identifiers

PostgreSQL identifiers cannot be treated like ordinary query values.

Any dynamic schema identifier MUST:

1. originate from `public.organizations`;
2. be treated as trusted internal state;
3. be safely identifier-quoted by the database adapter.

Do not do:

```go
fmt.Sprintf("SET search_path = %s", requestValue)
```

---

## 31. Team RLS

Team-owned tables MUST use RLS.

Example:

```sql
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks FORCE ROW LEVEL SECURITY;
```

The runtime role MUST not have `BYPASSRLS`. PostgreSQL explicitly documents that superusers, `BYPASSRLS` roles and normally table owners bypass row security.

---

## 32. RLS Context

Inside a team transaction:

```sql
SET LOCAL app.user_id = '54';
SET LOCAL app.team_id = '17';
```

A conceptual policy:

```sql
CREATE POLICY task_team_policy
ON tasks
USING (
    team_id =
        current_setting('app.team_id', true)::BIGINT
)
WITH CHECK (
    team_id =
        current_setting('app.team_id', true)::BIGINT
);
```

Both visibility and writes must be constrained.

---

## 33. RLS Purpose

RLS is defense in depth.

Application code still performs:

```text
authentication
organization membership
team membership
permission authorization
```

RLS exists so that a missing:

```sql
WHERE team_id = ...
```

does not automatically become a cross-team breach.

---

## 34. Tenant Executor

Feature developers MUST NOT manually manipulate `search_path`.

Provide a tenancy primitive.

Conceptually:

```go
tenant.WithTeam(ctx, tenantCtx, func(tx *TenantTx) error {
	...
	return nil
})
```

Internally:

```text
BEGIN
SET LOCAL search_path
SET LOCAL app.user_id
SET LOCAL app.team_id
create tenant sqlc queries
run callback
COMMIT
```

Rollback occurs automatically on failure.

---

## 35. Tenant Transaction Object

A useful abstraction is:

```go
type TenantTx struct {
	Queries *tenantdb.Queries
	Events  EventWriter
	Jobs    JobWriter
}
```

This allows services to perform transactional domain mutations without learning how tenant state is installed.

The raw helper that modifies tenant DB settings should remain inside:

```text
internal/tenancy
```

---

## 36. SQLC Separation

Generate shared and tenant query packages separately.

Conceptually:

```text
repository/publicdb
repository/tenantdb
```

Public queries:

```sql
SELECT ...
FROM public.organizations;
```

Tenant queries:

```sql
SELECT ...
FROM tasks;
```

Tenant tables are resolved through the transaction's safe search path.

This separation reduces accidental mixing of control-plane and tenant queries.

---

## 37. Internal IDs

Use:

```sql
id BIGINT GENERATED ... PRIMARY KEY
```

for:

```text
foreign keys
joins
indexes
internal cache references
```

---

## 38. Public IDs

Externally addressable resources use:

```sql
public_id UUID NOT NULL UNIQUE
```

URLs:

```text
/api/v1/orgs/{org_uuid}
/api/v1/orgs/{org_uuid}/teams/{team_uuid}
/api/v1/orgs/{org_uuid}/teams/{team_uuid}/tasks/{task_uuid}
```

Never:

```text
/tasks/78112
```

---

## 39. UUID Resolution

Resolve public UUIDs near the application boundary.

```text
UUID
 ↓
authorized lookup
 ↓
BIGINT
```

The majority of internal code should operate on numeric IDs.

---

## 40. Authentication Architecture

Synodus authenticates current browser users with local email/password credentials
and then creates a revocable opaque application session. Email comparison is
canonical and case-insensitive. Passwords use bounded Argon2id verification,
common-password blocking, uniform invalid-credential behavior, and bounded hash
concurrency.

Accounts are created by an operator command using an interactive prompt or
standard input. Public registration and email recovery are not current features.
Passwords MUST NOT appear in process arguments, logs, errors, or audit metadata.

New and changed passwords MUST contain 15–128 Unicode code points after NFC
normalization and at most 1024 UTF-8 bytes. Spaces and Unicode are permitted.
Reject a vendored common-password blocklist; do not require character-class
composition or periodic rotation. Encoded Argon2id parameters MUST be bounded
before decoding or allocating attacker-controlled sizes.

OIDC federation, MFA, passkeys, and non-browser API tokens are future extensions,
not current deployment dependencies. A future OIDC integration MUST use standard
libraries and current OAuth security guidance, link immutable issuer/subject
identity, and terminate in the same Synodus session and authorization model.

---

## 41. BFF Authentication Model

Use Backend-for-Frontend authentication.

```text
Browser
    │
    │ opaque HttpOnly cookie
    ▼
Synodus Go Backend
    │
    ├── bounded local credential verifier
    └── PostgreSQL session authority
```

Do not give browser JavaScript persistent bearer tokens.

Do not store authentication material such as:

```text
access token
refresh token
session token
```

in:

```text
localStorage
```

---

## 42. Authentication Flow

```text
POST /api/v1/auth/login
      ↓
canonicalize email
      ↓
perform one bounded Argon2id verification
      ↓
generate a fresh 256-bit session token
      ↓
store only SHA-256(token) with expiry/revocation state
      ↓
set secure cookie and return derived CSRF value
```

Unknown accounts use a dummy hash. Unknown email, wrong password, deleted account,
and unusable stored hash return the same invalid-credentials result. Accepted
outdated Argon2id parameters are upgraded after successful verification.

---

## 43. Application Session

Use a cryptographically random opaque session secret.

Browser:

```text
__Host-synodus_session=<random value>
```

The production cookie MUST be:

```text
Secure
HttpOnly
Path=/
SameSite=Lax
no Domain attribute
```

Database stores a hash of the token rather than its plaintext value.
Development uses a clearly separate non-secure cookie name that production
configuration rejects. The session has a 30-minute idle and 12-hour absolute
expiry.

---

## 44. Session Table

Conceptually:

```text
public.sessions
---------------
id
public_id
user_id
token_hash
created_at
last_seen_at
absolute_expires_at
revoked_at
user_agent_metadata
```

Never store the raw application session token. Bound user-agent metadata and
session inspection/cleanup queries. Idle validity is computed from
`last_seen_at`; absolute expiry is immutable.

---

## 45. Session Revocation

Support:

```text
logout current session
logout all sessions
administrator revocation
session expiry
account suspension
```

A user should be able to inspect active sessions.

---

## 46. MFA and Passkeys

Future identity extensions MAY provide:

```text
TOTP MFA
WebAuthn
passkeys
```

through a reviewed standards-based provider or maintained library.

WebAuthn provides origin/RP-scoped public-key credentials for strong web authentication.

Passkeys are an excellent stretch/demo authentication option, but the project does not need to implement WebAuthn itself.

---

## 47. CSRF

Because authentication uses cookies, unsafe requests require CSRF protection.

Required for unsafe cookie-authenticated requests:

```text
raw session token + dedicated server HMAC secret
        ↓
frontend receives CSRF token
        ↓
X-CSRF-Token header
        ↓
backend validates token
```

Also require an exact approved `Origin`; a valid CSRF header alone is insufficient.

---

## 48. Production CORS

Serve the web frontend and API under the same site when possible.

This largely eliminates production CORS complexity.

Development and production CORS MUST use validated exact allowlists. Production
MUST reject wildcard and insecure origins.

Never combine credentialed authentication with unrestricted origins.

---

## 49. Authorization

Authentication answers:

```text
Who are you?
```

Authorization answers:

```text
May you perform this operation?
```

Do not conflate them.

The local verifier authenticates identity. Synodus owns application sessions and
product authorization.

---

## 50. Organization Roles

Initial organization-level roles:

```text
owner
administrator
member
```

Owners manage owners and organization lifecycle. Administrators update the
organization and manage non-owner members. Members have read-only organization
access. Only owners and administrators create teams. Legacy organizations with
no explicitly assigned owner remain readable to current members but all
administrative mutations fail closed.

Organization administration MUST NOT grant team-content access.

---

## 51. Team Roles

Closed team roles:

```text
team_admin
research_lead
researcher
contributor
viewer
```

`team_admin` has team/member management and all task, file, audit, and realtime
permissions. `research_lead` has all task/file operations, audit read, and
realtime. `researcher` has task read/create/update/move, file read/upload, and
realtime. `contributor` omits task move. `viewer` has only team/task/file read
and realtime.

---

## 52. Permissions

The M03-M05 permission vocabulary begins with:

```text
org.read
org.update
org.delete
org.restore
org.members.read
org.members.manage
org.owners.manage
team.create
team.read
team.update
team.delete
team.members.read
team.members.manage
task.read
task.create
task.update
task.move
task.delete
file.read
file.upload
file.delete
audit.read
realtime.connect
```

Later milestones extend typed permissions for their domains. Do not spread role
string comparisons throughout services. Unknown roles and permissions deny.

---

## 53. Authorization Service

Feature code SHOULD call a centralized authorization abstraction.

For example:

```go
authorizer.Require(
	ctx,
	permission.TaskUpdate,
	resource,
)
```

The authorizer can evaluate:

```text
organization membership
team membership
role
permission
resource ownership
special policy
```

---

## 54. Authorization Layers

Canonical security path:

```text
application session
      ↓
organization membership
      ↓
team membership
      ↓
permission
      ↓
tenant transaction
      ↓
PostgreSQL RLS
```

Every layer provides a separate failure boundary.

---

## 55. API Architecture

Use:

```text
/api/v1
```

Do not expose internal implementation details in URLs.

---

## 56. OpenAPI

Maintain:

```text
api/openapi.yaml
```

as the HTTP contract.

Use it to:

* document endpoints;
* validate schemas;
* generate TypeScript client types;
* detect breaking API changes;
* drive API review.

OpenAPI exists specifically to provide a language-independent machine-readable HTTP API description.

---

## 57. API Client Generation

Generate the frontend client from OpenAPI.

Do not manually maintain duplicate TypeScript interfaces for every API DTO.

Generated code MUST NOT be manually edited.

---

## 58. API Error Format

Use:

```text
Content-Type: application/problem+json
```

following RFC 9457.

Example:

```json
{
  "type": "/problems/precondition-failed",
  "title": "Resource version conflict",
  "status": 412,
  "detail": "The resource changed since it was read.",
  "request_id": "5d6...",
  "violations": []
}
```

Never send raw PostgreSQL errors to clients.
Every error response, including middleware and router failures, uses
`application/problem+json`. Session failures are `401`, known operation-level
permission failures are `403`, and missing versus unauthorized protected IDs
share an indistinguishable `404` representation.

---

## 59. Error Taxonomy

Services should distinguish:

```text
invalid input
unauthenticated
permission denied
not found
conflict
precondition failed
rate limited
dependency unavailable
internal failure
```

Handlers map domain/application errors to HTTP.

---

## 60. Pagination

Every potentially large list endpoint MUST be bounded.

Use cursor/keyset pagination for:

```text
chat history
tasks
audit entries
notifications
organization members
artifacts
experiments
```

The default page size is 50 and the maximum is 100. Example:

```text
GET /messages?limit=50&cursor=...
```

Server controls the maximum page size.

---

## 61. Cursor Tokens

Cursor values SHOULD be opaque to clients.

Example internal cursor:

```text
created_at
id
```

encoded and integrity-protected by the API.

Use a bounded HMAC-signed token scoped to its route, organization/team public
IDs, sort tuple, direction, version, and expiry. Do not expose internal IDs or
accept a cursor on a different route/scope. Do not use uncontrolled user-supplied
SQL offsets for massive tables.

---

## 62. Optimistic Concurrency

Collaborative mutable resources MUST protect against lost updates.

Use:

```text
version BIGINT NOT NULL
```

Example:

```sql
UPDATE tasks
SET
    title = $1,
    version = version + 1
WHERE id = $2
  AND version = $3;
```

Zero rows updated:

```text
concurrent modification
```

---

## 63. HTTP Conditional Updates

Expose resource versions through:

```text
ETag
```

and accept:

```text
If-Match
```

for important mutable resources.

HTTP defines conditional request semantics through the standard HTTP model.

A stale client SHOULD receive:

```text
412 Precondition Failed
```

rather than silently overwriting another user's work.

---

## 64. Idempotency

Operations likely to be retried SHOULD accept:

```text
Idempotency-Key
```

Examples:

```text
organization creation
upload initiation
resource booking
artifact publication
AI generation
```

Store:

```text
principal
operation
key hash
resource/result
expiry
```

Retries must not produce duplicates.

---

## 65. Task System

Tasks SHOULD support:

```text
title
description
status
priority
assignees
labels
due date
project
version
created_by
created_at
updated_at
```

Do not implement Jira-scale complexity.

---

## 66. Kanban Ordering

Avoid renumbering an entire column for every drag operation.

A practical design:

```text
rank BIGINT
```

with gaps:

```text
1024
2048
3072
...
```

Moving between two tasks can assign a rank between them.

Rebalance only when required.

Concurrent board writes still use resource versions.

---

## 67. Chat

Chat state is durable PostgreSQL data.

Model:

```text
Team
 └── Channels
      └── Messages
```

Messages use cursor pagination.

Edits should retain:

```text
edited_at
version
```

Deletion SHOULD use a tombstone rather than rewriting chat history invisibly.

---

## 68. Realtime Architecture

The preferred realtime infrastructure is **Centrifugo**, rather than requiring the student team to engineer an entire production WebSocket broker.

Centrifugo supports bounded history/recovery semantics and Redis-backed engines suitable for multi-process realtime delivery.

---

## 69. Realtime Trust Model

The Go backend remains responsible for:

```text
authentication
authorization
subscription decisions
domain state
```

Centrifugo transports events.

It does not become the authorization database.

---

## 70. Realtime Channels

Use public IDs.

Example:

```text
team:<orgUUID>:<teamUUID>
chat:<orgUUID>:<teamUUID>:<channelUUID>
task:<orgUUID>:<teamUUID>
document:<orgUUID>:<teamUUID>:<docUUID>
```

Never expose internal BIGINT IDs through realtime identifiers.

---

## 71. Subscription Authorization

A client MUST NOT subscribe to an arbitrary team channel merely by knowing its name.

Subscription authorization must consult Synodus membership/permission state.

---

## 72. Realtime Event Contract

Maintain a common event envelope.

```json
{
  "event_id": "uuid",
  "type": "task.updated",
  "schema_version": 1,
  "occurred_at": "2026-08-18T07:20:00Z",
  "organization_id": "uuid",
  "team_id": "uuid",
  "resource_id": "uuid",
  "resource_version": 19,
  "data": {}
}
```

---

## 73. AsyncAPI

Maintain:

```text
api/asyncapi.yaml
```

for realtime contracts.

Document:

```text
channels
events
payloads
event versions
authorization expectations
```

Realtime protocols deserve the same discipline as REST APIs.

---

## 74. Realtime State Authority

Realtime events are notifications of state changes.

They are not authoritative storage.

Canonical rule:

```text
PostgreSQL = truth

realtime event = notification
```

If realtime is lost, clients must recover via REST.

---

## 75. Reconnect

After WebSocket/realtime recovery fails:

```text
reconnect
   ↓
reauthorize
   ↓
fetch authoritative resource state
   ↓
resume realtime
```

Centrifugo history may recover short gaps, but the application database remains authoritative. Its history feature is intentionally bounded.

---

## 76. Durable Background Work

Important work MUST NOT run as detached goroutines from HTTP handlers.

Use durable jobs.

The recommended Go queue is **River**, backed by PostgreSQL.

River supports transaction-safe job insertion, allowing the application mutation and job creation to participate in one database transaction.

---

## 77. River Schema

Place River's own tables in a dedicated schema such as:

```text
river
```

River supports explicitly selecting a PostgreSQL schema for its internal tables.

Do not allow tenant `search_path` behavior to accidentally select River objects.

---

## 78. Transactional Side Effects

Instead of:

```text
UPDATE task
COMMIT

publish realtime
```

use:

```text
BEGIN

UPDATE tenant task

INSERT durable PublishTaskUpdated job

COMMIT
```

If the transaction rolls back, the event job disappears too.

If it commits, processing survives an API process crash.

---

## 79. Job Categories

Use durable workers for:

```text
realtime publication
email notifications
AI processing
file cleanup
artifact metadata extraction
key rotation workflows
document compaction
expired-session cleanup
maintenance operations
```

---

## 80. At-Least-Once Semantics

Assume jobs may execute more than once.

Every worker handler MUST be idempotent or otherwise duplicate-safe.

Do not claim exactly-once execution.

---

## 81. Retry Policy

Classify failures:

```text
temporary
permanent
```

Temporary errors may retry with:

```text
bounded exponential backoff
+
jitter
```

Permanent errors should stop retrying.

Failed jobs must remain operationally visible.

---

## 82. Redis Responsibilities

Redis should have narrowly defined responsibilities:

```text
Centrifugo engine
rate limiting
ephemeral presence/state
selected caches
```

PostgreSQL remains authoritative.

Login uses Redis-backed GCRA with both an HMAC-pseudonymous canonical-account
bucket and a trusted-client-IP bucket. Defaults are five failures per account per
15 minutes and 20 attempts per client IP per 15 minutes; successful login clears
the account-failure bucket. Redis keys MUST NOT contain raw email or IP values.

---

## 83. Redis Is Not Durable Domain Storage

Do not put authoritative copies of:

```text
tasks
memberships
documents
permissions
audit records
research artifacts
```

only in Redis.

---

## 84. Caching

Do not add caching automatically.

Before caching, document:

```text
what query is slow?
what is the source of truth?
what is the cache key?
what invalidates it?
what stale state is acceptable?
what happens if Redis fails?
```

Only then introduce caching.

---

## 85. Redis Failure Behavior

Expected:

```text
Redis cache failure
→ PostgreSQL fallback

Redis realtime failure
→ realtime degradation
→ persistent CRUD remains correct

Redis presence failure
→ presence unavailable
→ collaboration state remains correct

Redis rate-limit failure
→ login and administrative mutations return a bounded 503
→ ordinary authenticated requests continue through PostgreSQL authorization
```

---

## 86. File Storage

MinIO stores binary objects.

PostgreSQL stores:

```text
ownership
team
object identifier
state
size
metadata
encryption metadata
created_at
```

Do not store large research files as PostgreSQL `BYTEA`.

---

## 87. Upload Architecture

Use direct object upload.

```text
Browser
   │
   │ request upload
   ▼
Go API
   │
authorize
create upload record
generate object key
generate presigned URL
   │
   ▼
Browser
   │
   │ upload
   ▼
MinIO
   │
   ▼
complete API
```

This avoids routing large object bodies through the Go API.

---

## 88. File Lifecycle

Persist a state machine.

```text
INITIATED
    ↓
UPLOADING
    ↓
VERIFYING
    ↓
AVAILABLE

or

FAILED

AVAILABLE
    ↓
DELETING
    ↓
DELETED
```

External storage and PostgreSQL do not participate in one distributed transaction, so intermediate states must be modeled explicitly.

---

## 89. Object Keys

Object keys MUST be generated by trusted server code.

Prefer opaque keys.

Example:

```text
objects/3f/3f85f70e-....
```

Do not let clients submit arbitrary MinIO paths.

Object path naming is not a security boundary.

Authorization happens before generating signed access.

---

## 90. Multipart Upload

Large objects SHOULD use multipart upload.

This provides manageable retries and prevents a single failed transfer from restarting the entire object.

Upload sessions must expire and abandoned multipart uploads must be cleaned asynchronously.

---

## 91. Privacy Modes

Synodus MUST distinguish between content that the server may process and content intentionally hidden from the server.

### Managed content

Server may process plaintext.

Supports:

```text
AI
search
preview
indexing
collaborative server processing
```

### Confidential content

Client encrypts content before upload.

Server stores ciphertext.

Some server features necessarily become unavailable.

---

## 92. Do Not Make Impossible Privacy Claims

Do not claim simultaneously:

```text
server cannot read data

and

server performs plaintext AI/search over that same data
```

without explicitly explaining how plaintext becomes available.

The threat model must remain coherent.

---

## 93. Confidential File Encryption

Confidential files SHOULD be encrypted client-side using authenticated encryption.

The existing Synodus direction of:

```text
AES-256-GCM
```

is appropriate.

Keys and nonces MUST be generated using cryptographically secure randomness.

Nonce reuse under the same key is prohibited.

---

## 94. Chunked File Encryption

Large files SHOULD be encrypted incrementally rather than loaded entirely into memory.

Conceptually:

```text
file DEK
  │
  ├── unique nonce → chunk 0
  ├── unique nonce → chunk 1
  ├── unique nonce → chunk 2
  └── ...
```

The encrypted file format MUST contain a version so that cryptographic formats can evolve.

Example:

```text
magic
format version
algorithm identifier
chunk size
file identifier
chunk records
```

---

## 95. Envelope Encryption

Each confidential file gets its own random:

```text
Data Encryption Key (DEK)
```

```text
DEK
 │
 └── encrypts file
```

The DEK itself is wrapped by the team's key hierarchy.

Do not use one encryption key directly for every team file.

---

## 96. Team Key Versions

Team key state is versioned.

```text
team key v1
team key v2
team key v3
```

Files record which key version wraps their DEK.

---

## 97. Membership Revocation

When a member is removed:

```text
rotate team key
      ↓
new content uses new key version
      ↓
removed member receives no new wrapped key
```

Do not claim that rotation can erase plaintext or keys the removed member already possessed.

That limitation must be documented accurately.

---

## 98. Cryptographic Engineering Rule

Do not invent new cryptographic primitives.

The crypto subsystem requires:

```text
separate ADR
documented threat model
versioned formats
test vectors
tamper tests
key-rotation tests
review by at least two team members
```

Cryptographic code should be isolated from ordinary UI/business logic.

---

## 99. Documents

Collaborative documents use:

```text
Tiptap
+
Yjs
```

Do not build a custom CRDT.

Persist:

```text
document metadata
Yjs updates
periodic snapshots
```

Periodically compact long update histories.

---

## 100. Confidential Documents

Encrypted collaborative documents are significantly more difficult because search, server persistence and realtime merge behavior change.

Therefore:

```text
managed collaborative documents
    = core feature

fully confidential collaborative documents
    = stretch feature
```

Do not weaken the entire system trying to solve every cryptographic collaboration problem simultaneously.

---

## 101. Research Artifact Registry

Synodus should distinguish:

```text
File
```

from:

```text
Research Artifact
```

A file is storage.

An artifact represents research meaning.

---

## 102. Artifact Model

Examples:

```text
dataset
benchmark result
trained model
simulation output
compiled binary
experiment log
figure
paper supplement
configuration
container image reference
```

---

## 103. Artifact Versions

Published artifact versions SHOULD be immutable.

Example:

```text
Artifact: RDMA Benchmark Results

v1
v2
v3
```

Changes create a new version.

Do not silently modify previously published research results.

---

## 104. Artifact Metadata

A version may record:

```text
creator
created_at
content hash
source file
source Git commit
container image digest
experiment
parameters
input artifact versions
environment metadata
```

---

## 105. Experiment Registry

Experiments SHOULD record:

```text
name
description
creator
status
source revision
parameters
input artifacts
output artifacts
started_at
finished_at
environment
```

Synodus does not need to execute the experiment.

Its responsibility is to record and connect it.

---

## 106. Research Provenance

Research relationships form a graph.

```text
Dataset v4
    │
    ▼
Experiment 21
    │
    ├──► results.csv
    └──► model.bin
              │
              ▼
         Experiment 27
              │
              ▼
           Figure 8
```

The UI SHOULD make this graph inspectable.

This is a major differentiator for Synodus.

---

## 107. Content Hashes

Published research artifact versions SHOULD carry cryptographic content hashes where applicable.

This supports:

```text
integrity verification
reproducibility
comparison
backup verification
```

---

## 108. Resource Booking

Research equipment/resources may include:

```text
GPU server
microscope
laboratory room
measurement equipment
test rig
```

Booking conflicts MUST be enforced by PostgreSQL rather than relying only on frontend availability checks.

---

## 109. Booking Concurrency

Two concurrent requests attempting to reserve the same resource for an overlapping interval must not both succeed.

Use database constraints/transactions appropriate to PostgreSQL.

This should become one of the project's explicit concurrency demonstrations.

---

## 110. Notifications

Notifications are durable application state.

Examples:

```text
task assigned
mention
team invitation
artifact published
booking conflict/change
AI result ready
```

Realtime merely accelerates delivery.

---

## 111. AI Architecture

The AI service MUST NOT receive unrestricted database credentials.

Canonical path:

```text
User
 │
 ▼
Go API
 │
 ├── authenticate
 ├── authorize
 ├── select permitted resources
 ├── minimize context
 │
 ▼
Python AI Service
```

The Go application remains the authorization enforcement point.

---

## 112. AI Capabilities

Reasonable capabilities:

```text
document summarization
chat summarization
meeting-note summarization
action-item extraction
task suggestions
experiment comparison
research context retrieval
```

---

## 113. AI Actions

AI SHOULD propose important state changes rather than silently execute them.

Example:

```text
meeting notes
      ↓
AI
      ↓
suggested tasks
      ↓
user review
      ↓
approve
      ↓
tasks created
```

---

## 114. AI and Confidential Data

Confidential content MUST NOT silently be decrypted for AI.

If an explicit feature permits AI processing of decrypted confidential content:

1. the user must explicitly request it;
2. the UI must disclose that plaintext will be exposed to the AI processing boundary;
3. only selected content should be sent;
4. persistence should be minimized.

---

## 115. AI Security

Treat retrieved documents as untrusted model input.

Do not interpret text inside a user document as privileged system instructions.

Tool access should be:

```text
allowlisted
authorized
bounded
audited
```

AI output must be validated before becoming application state.

The OWASP LLM verification work can provide an additional checklist for the AI subsystem.

---

## 116. Search

Do not introduce Elasticsearch merely because Synodus has search.

Start with PostgreSQL:

```text
full-text search
GIN
pg_trgm
```

for manageable plaintext metadata/content.

Introduce external search only after measured limitations.

---

## 117. Confidential Search

Server-side plaintext indexing is incompatible with server-inaccessible encrypted content.

Confidential files may therefore support:

```text
metadata search
```

without supporting:

```text
server plaintext full-text search
```

unless the privacy model is deliberately changed.

---

## 118. Audit Architecture

Maintain two classes.

### Global security audit

Shared:

```text
authentication
organization lifecycle
organization membership changes
administrative session actions
```

### Tenant audit

Per organization:

```text
team administration
permission changes
file access
artifact publication
experiment changes
resource bookings
sensitive deletions
AI invocation
```

---

## 119. Audit Record

Conceptually:

```text
event_id
timestamp
actor
action
organization
team
resource_type
resource_public_id
result
trace_id
metadata
```

---

## 120. Audit Mutability

Normal runtime code SHOULD have:

```text
INSERT
SELECT
```

where required,

but not arbitrary:

```text
UPDATE
DELETE
```

on audit records.

Audit history should be append-oriented.

---

## 121. Audit Privacy

Never put the following into audit metadata:

```text
password
session token
federated identity token if later enabled
encryption key
private key
full chat body
document plaintext
confidential file contents
```

Audit means accountability, not indiscriminate data duplication.

---

## 122. Tamper-Evident Audit

A hash-chained audit checkpoint system is a useful stretch objective.

```text
H1 = Hash(H0 || event1)
H2 = Hash(H1 || event2)
...
```

This makes certain later modifications detectable relative to trusted checkpoints.

It does not make database compromise impossible.

---

## 123. Logging

Use Go:

```text
log/slog
```

with structured JSON output.

Core fields:

```text
timestamp
level
service
environment
request_id
trace_id
method
route
status
duration
```

Safe contextual IDs may also be attached.

---

## 124. Logging Rules

Never log:

```text
passwords
session cookies
Authorization headers
federated identity tokens if later enabled
CSRF secrets
private keys
file DEKs
team encryption keys
confidential document contents
```

---

## 125. Logging Errors

Log actual internal failures.

Example:

```go
slog.ErrorContext(
	ctx,
	"create task failed",
	"error", err,
)
```

Client validation failures generally do not deserve `ERROR`.

---

## 126. Error Ownership

Prefer logging an error once near the outer operational boundary.

Lower layers should wrap errors:

```go
return fmt.Errorf("insert task: %w", err)
```

Do not produce the same stack of identical ERROR logs from repository, service and handler.

---

## 127. Observability

Instrument Synodus using OpenTelemetry.

OpenTelemetry provides a common framework for traces, metrics and logs and supports collection/export through the Collector.

---

## 128. Distributed Tracing

Trace important flows.

Examples:

```text
HTTP
 ↓
service
 ↓
tenant executor
 ↓
PostgreSQL
```

and:

```text
HTTP
 ↓
transaction
 ↓
River job
 ↓
worker
 ↓
AI
```

and:

```text
task mutation
 ↓
durable event job
 ↓
Centrifugo
```

---

## 129. Metrics

Minimum application metrics:

```text
request rate
request errors
request duration

DB query duration
DB pool utilization

job queue depth
job failures
job latency

realtime publication failures
active realtime connections

MinIO latency
upload failures

AI latency
AI failures
```

---

## 130. Metric Cardinality

Do not use:

```text
user_id
task_id
request_id
file_id
```

as Prometheus metric labels.

These produce unbounded cardinality.

Such identifiers belong in traces/logs instead.

---

## 131. PostgreSQL Observability

Enable:

```text
pg_stat_statements
```

for performance investigation.

PostgreSQL documents it as tracking planning and execution statistics for SQL statements.

Use it to identify expensive query patterns.

---

## 132. Health Endpoints

Expose:

```text
/health/live
/health/ready
```

Liveness:

```text
Is this process functioning?
```

Readiness:

```text
Can this instance serve its primary workload?
```

Do not make liveness dependent on every optional subsystem.

---

## 133. Graceful Shutdown

API:

```text
SIGTERM
   ↓
mark not ready
   ↓
stop accepting new work
   ↓
drain requests
   ↓
close realtime resources
   ↓
close dependencies
```

Worker:

```text
stop acquiring work
   ↓
finish bounded current work
   ↓
release
   ↓
exit
```

---

## 134. Timeouts

Every external operation requires a deadline.

Examples:

```text
PostgreSQL
Redis
MinIO
AI
SMTP
Centrifugo
```

No dependency call should block indefinitely.

---

## 135. Backpressure

Unbounded queues are prohibited.

Bound:

```text
worker concurrency
HTTP request body size
realtime outgoing queue
AI concurrency
database pool size
upload metadata processing
```

Overload should cause controlled rejection or delayed work, not memory exhaustion.

---

## 136. HTTP Server Hardening

Configure:

```text
ReadHeaderTimeout
ReadTimeout
WriteTimeout
IdleTimeout
MaxHeaderBytes = 32 KiB
route-specific request size limits
```

Use route-specific body limits.

Trust forwarded client/protocol headers only from explicitly configured proxy
CIDRs and bound the parsed hop count. Otherwise use the direct peer and ignore
forwarded values.

Do not permit a normal JSON endpoint to accept arbitrary gigabyte-sized bodies.

Large files bypass the API using MinIO direct transfer.

---

## 137. Security Headers

Production SHOULD configure:

```text
Content-Security-Policy
Strict-Transport-Security
X-Content-Type-Options
Referrer-Policy
Permissions-Policy
frame-ancestors
```

Policies must match the actual application rather than being copied blindly.

---

## 138. Threat Modeling

Maintain:

```text
security/threat-model.md
```

Include:

```text
assets
attackers
trust boundaries
entry points
abuse cases
mitigations
residual risk
```

---

## 139. Threat Actors

Explicitly model:

```text
anonymous Internet attacker
malicious organization member
malicious team member
member of another team
member of another organization
stolen browser session
removed collaborator
malicious uploaded data
compromised AI integration
stolen database backup
stolen MinIO storage
misconfigured administrator
```

---

## 140. Trust Boundaries

Document at least:

```text
Browser ↔ API

local credentials/session store ↔ API

API ↔ PostgreSQL

API ↔ Redis

API ↔ MinIO

API ↔ Centrifugo

API ↔ AI

Organization A ↔ Organization B

Team A ↔ Team B

Managed ↔ Confidential content
```

---

## 141. ASVS Verification

Maintain:

```text
security/asvs.md
```

Map relevant OWASP ASVS 5.0.0 controls to:

```text
implemented
not applicable
planned
evidence/test
```

ASVS is intended as a concrete application-security requirement and verification framework, not merely a label for a report.

---

## 142. Database Security Tests

Explicitly test:

```text
runtime cannot CREATE SCHEMA

runtime cannot ALTER tenant tables

runtime cannot bypass RLS

tenant schema comes only from trusted organization mapping

cross-organization requests cannot switch schema

cross-team SELECT blocked

cross-team UPDATE blocked

cross-team DELETE blocked

cross-team INSERT blocked
```

---

## 143. Connection Pool Isolation Test

Exercise thousands of alternating operations:

```text
Org A / Team 1
Org B / Team 4
Org A / Team 2
Org C / Team 3
...
```

through `pgxpool`.

After every transaction verify:

```text
no search_path leakage
no app.team_id leakage
no app.user_id leakage
```

This is one of the project's most important security integration tests.

---

## 144. Deliberately Broken Query Test

Create an integration test using:

```sql
SELECT *
FROM tasks;
```

without:

```sql
WHERE team_id = ...
```

The result MUST still contain only the active team because RLS is enforcing the boundary.

This is valuable evidence during evaluation.

---

## 145. Unit Testing

Unit tests target:

```text
business rules
permission mapping
state transitions
event creation
cursor logic
validation
key metadata logic
```

External infrastructure should be mocked/faked only when infrastructure behavior itself is not what is being tested.

---

## 146. PostgreSQL Integration Tests

Concrete database logic MUST run against real PostgreSQL.

Test:

```text
sqlc queries
transactions
constraints
RLS
locks
migration behavior
concurrent operations
```

SQLite is not an acceptable substitute for PostgreSQL integration testing.

---

## 147. Infrastructure Integration Tests

Use real instances for:

```text
Redis
MinIO
PostgreSQL
```

when testing their concrete adapters.

Testcontainers-Go or isolated Compose environments are appropriate.

---

## 148. Concurrency Tests

Required examples:

### Task concurrency

```text
Client A: version 9
Client B: version 9

A writes
B writes
```

Expected:

```text
one succeeds
one receives conflict
```

### Booking concurrency

```text
two overlapping bookings
```

Expected:

```text
one accepted
one rejected
```

---

## 149. Worker Failure Testing

Example:

```text
commit task update
      ↓
durable event job exists
      ↓
kill worker
      ↓
restart
      ↓
event processed
```

This must work.

---

## 150. Duplicate Job Test

Execute the same worker operation twice.

State must remain correct.

Examples:

```text
duplicate notification publication
duplicate cleanup job
duplicate artifact metadata extraction
```

---

## 151. Failure Injection

Build repeatable tests for:

```text
Redis unavailable
MinIO unavailable
AI timeout
Centrifugo unavailable
worker terminated
API terminated
database transaction rollback
upload interrupted
duplicate HTTP request
```

Document expected degradation.

---

## 152. Fuzz Testing

Use native Go fuzz testing at security-sensitive parsing boundaries.

Good candidates:

```text
cursor decoder
UUID/request parsers
event decoder
encrypted file manifest parser
import parser
```

---

## 153. Cryptographic Tests

Must include:

```text
encrypt → decrypt roundtrip

ciphertext modification rejected

wrong key rejected

nonce uniqueness strategy tested

key version rotation

removed member receives no new key

file-format version parser

large chunked file roundtrip
```

---

## 154. End-to-End Tests

E2E should verify major user journeys:

```text
login
create organization
provision organization
create team
invite member
create task
concurrent update
upload confidential file
download/decrypt file
publish artifact
record experiment
book resource
receive realtime event
```

---

## 155. Performance Testing

Use a dedicated load tool such as:

```text
k6
```

Test representative workloads rather than synthetic `/ping`.

Examples:

```text
task list
task mutation
chat history
artifact search
session validation
realtime fanout
```

---

## 156. Initial Performance Objectives

These are engineering targets, not commercial SLAs.

Suggested starting objectives under a documented reference load:

```text
ordinary CRUD:
p95 < 250 ms

task realtime propagation:
p95 < 500 ms

API error rate:
< 1% under expected load

tenant isolation failures:
0

lost committed durable jobs:
0
```

Adjust only based on measured results.

---

## 157. Profiling

Use Go:

```text
pprof
```

to investigate:

```text
CPU
heap
allocations
goroutines
mutex contention
blocking
```

At least one measured optimization should appear in the final engineering report.

pprof MUST NOT be mounted on the public API router. When explicitly enabled
outside production, it binds through a separate loopback-only diagnostic server
with bounded timeouts.

---

## 158. Query Performance

Important queries SHOULD be inspected with:

```sql
EXPLAIN (ANALYZE, BUFFERS)
```

Record:

```text
query
data size
old plan
problem
change
new plan
measured difference
```

This is stronger than simply claiming the database is optimized.

---

## 159. Indexing Policy

Indexes MUST correspond to actual access patterns.

Likely candidates:

```text
public UUID lookup

team_id + status

channel_id + created_at + id

team membership lookups

artifact version lookup

experiment relationships

booking resource + time
```

Do not index every column.

---

## 160. Transaction Philosophy

Use PostgreSQL transactions for real consistency invariants.

Examples:

```text
task update + durable event

membership modification + key rotation job

artifact publication

resource booking

organization control-plane mutation
```

Transactions should be short.

---

## 161. External Resources and Transactions

Never:

```text
BEGIN
upload 2GB
COMMIT
```

because MinIO is outside the PostgreSQL transaction.

Use durable state machines instead.

---

## 162. Application Mutexes

Go mutexes MAY protect process-local structures.

They MUST NOT be used as the correctness mechanism for state shared across API replicas.

For distributed correctness use:

```text
database constraints
transactions
row locks
optimistic versions
advisory locks
```

---

## 163. Configuration

Centralize configuration.

Example:

```go
type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Redis         RedisConfig
	MinIO         MinIOConfig
	Auth          AuthConfig
	Realtime      RealtimeConfig
	AI            AIConfig
	Telemetry     TelemetryConfig
}
```

Validate all mandatory configuration at startup.

---

## 164. Secrets

Never commit:

```text
database passwords
session, CSRF, cursor, and rate-limit HMAC secrets
MinIO credentials
SMTP credentials
AI credentials
private encryption material
```

Development credentials may exist only as clearly non-production defaults.

---

## 165. Docker Images

Application containers SHOULD:

```text
run as non-root
use multi-stage builds
contain only required runtime files
have health checks where appropriate
avoid shell/tooling in minimal runtime image where practical
```

Do not sacrifice debuggability blindly for image size.

---

## 166. Self-Hosted Deployment

Official self-hosted deployment SHOULD use:

```text
Docker Compose
```

This is the supported reference environment.

Kubernetes is not required.

---

## 167. Production-Like Compose Topology

A high-quality demo environment:

```text
caddy

web

api-1
api-2

worker-1
worker-2

provisioner

centrifugo-1
centrifugo-2

postgres
redis
minio

ai

otel-collector
prometheus
grafana
tempo
loki
```

Two replicas demonstrate that the application does not depend on process-local business state.

---

## 168. Do Not Misrepresent Availability

Two API replicas do not make the entire platform highly available when PostgreSQL or MinIO remains single-node.

Describe the deployment accurately:

```text
replicated stateless application layer

single-node persistence layer
```

unless actual storage HA is implemented.

---

## 169. BYO Infrastructure (Stretch Goal)

Synodus may permit compatible externally managed:

```text
PostgreSQL
Redis
S3-compatible storage
```

through configuration.

A future federation extension may separately support a compatible OIDC provider;
it is not part of the current reference topology.

But "bring your own database" is not equivalent to self-hostability.

The entire application stack must be deployable by the operator for Synodus to be self-hostable.

---

## 170. Backup

Minimum backup plan:

```text
PostgreSQL backup
MinIO backup/mirror
configuration backup
critical secret recovery procedure
```

Backups SHOULD themselves be encrypted/access-controlled.

---

## 171. Restore Drill

A backup strategy is incomplete until recovery is tested.

Project demonstration:

```text
create organization
create team
create tasks
upload files
publish artifact
record hashes
      ↓
backup
      ↓
destroy environment
      ↓
restore
      ↓
verify all expected state/hashes
```

Perform this before final evaluation.

---

## 172. Recovery Objectives

Document approximate project:

```text
RPO
RTO
```

even if the implementation only provides basic scheduled backups.

Do not claim zero data loss without infrastructure that provides it.

---

## 173. Operational CLI

`cmd/admin` SHOULD provide a small operational CLI.

Useful commands:

```text
org list
org inspect
org provisioning retry

tenant migration status

user sessions
session revoke

jobs failed
jobs retry

storage verify

audit inspect
```

This is more useful than building a large admin web dashboard.

---

## 174. Runbooks

Maintain operational documentation for:

```text
PostgreSQL unavailable
Redis unavailable
MinIO unavailable
credential/session incident
AI outage
failed tenant migration
stuck durable jobs
failed organization provisioning
restore from backup
suspected tenant leak
credential rotation
```

---

## 175. Architecture Decision Records

Significant decisions require ADRs.

Recommended initial ADRs:

```text
ADR-001 Modular monolith

ADR-002 Schema-per-organization tenancy

ADR-003 Team RLS

ADR-004 BIGINT internal + UUID external IDs

ADR-005 Local password/BFF session authentication

ADR-006 PostgreSQL durable jobs

ADR-007 Centrifugo realtime

ADR-008 Client-side encrypted file vault

ADR-009 Research artifact provenance

ADR-010 OpenTelemetry observability
```

ADR template:

```text
Context
Decision
Alternatives
Consequences
Status
```

---

## 176. CI Pipeline

Required pipeline should approximately be:

```text
format
   ↓
build
   ↓
unit tests
   ↓
Go race detector
   ↓
integration tests
   ↓
tenant isolation tests
   ↓
golangci-lint
   ↓
gosec
   ↓
govulncheck
   ↓
frontend lint/test
   ↓
migration validation
   ↓
OpenAPI validation
   ↓
container build
   ↓
container vulnerability scan
   ↓
SBOM generation
```

---

## 177. Additional Security CI

Strong additions:

```text
secret scanning
CodeQL
dependency review
container scanning
SBOM
signed release images
```

Do not run ten overlapping scanners merely to generate badges.

Every check should have an understood purpose.

---

## 178. Generated Files

Generated code MUST NOT be manually edited.

Examples:

```text
sqlc
OpenAPI client
mock code where generation is used
```

CI SHOULD detect uncommitted generated changes.

---

## 179. Dependency Management

Dependencies should be deliberately pinned.

Before introducing a dependency ask:

```text
What problem does it solve?

Can the standard library solve it?

Is it actively maintained?

What security/operational boundary does it create?

Can we replace it later?
```

Do not minimize dependency count at the cost of reimplementing complex infrastructure badly.

---

## 180. Main Function

`main()` should remain small.

Example:

```go
func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		slog.Error("application terminated", "error", err)
		os.Exit(1)
	}
}
```

`run()` handles dependency construction and application lifecycle.

---

## 181. Dependency Wiring

Construction flow:

```text
Config
  ↓
PostgreSQL
Redis
MinIO
local identity/session services
Realtime client
Telemetry
  ↓
repositories/executors
  ↓
authorization
services
  ↓
handlers
  ↓
HTTP server
```

Avoid global service singletons.

---

## 182. Context

Every request/I/O path accepts:

```go
context.Context
```

as the first parameter.

Do not store request contexts inside long-lived service objects.

---

## 183. Interfaces

Create interfaces where substitution or boundary ownership is useful.

Good:

```text
FileStore
Authorizer
AIClient
EventPublisher
TenantExecutor
```

Do not create an interface for every concrete struct automatically.

Interfaces should normally be defined by the consumer.

---

## 184. Go Error Style

Wrap errors:

```go
return fmt.Errorf("create task: %w", err)
```

Use:

```text
errors.Is
errors.As
```

for typed decisions.

Avoid string comparison for error classification.

---

## 185. Handler Responsibilities

Handlers may:

```text
decode
validate request shape
extract route IDs
call application service
map errors
write response
```

Handlers MUST NOT contain:

```text
SQL
RLS configuration
Redis commands
MinIO workflows
authorization algorithms
complex business workflows
```

---

## 186. Service Responsibilities

Services contain:

```text
business policy
authorization orchestration
transaction boundaries
domain transitions
cross-adapter coordination
```

A service operation should describe an application use case.

Good:

```text
CreateTask
PublishArtifact
BookResource
RemoveTeamMember
CompleteUpload
```

---

## 187. Repository Responsibilities

Repositories/query adapters deal with persistence.

They must not decide:

```text
whether a user is allowed to perform a business operation
```

unless the database operation is itself implementing an explicit security invariant such as RLS.

---

## 188. Development Strategy

Build vertical slices.

Do not create the entire eventual schema at once.

For each feature:

```text
requirement
   ↓
threat/security analysis
   ↓
migration
   ↓
sqlc
   ↓
service/domain
   ↓
authorization
   ↓
HTTP API
   ↓
async/realtime effects
   ↓
tests
   ↓
telemetry
```

---

## 189. Before Adding a Table

Answer:

```text
Which current feature requires it?

public or tenant schema?

Does it belong to a team?

Does it require RLS?

Who owns the data?

Which constraints enforce correctness?

Which queries require indexes?

Does it need soft deletion?

Does it need versioning?
```

If there is no current feature requiring it, do not add it yet.

---

## 190. Before Adding Redis

Answer:

```text
Why PostgreSQL is insufficient here?

Is the state durable or ephemeral?

What happens if Redis disappears?

Can stale data cause authorization errors?

How is invalidation handled?
```

Never cache authorization data casually.

---

## 191. Before Adding a Background Job

Answer:

```text
Why can't it complete synchronously?

Must it survive process failure?

Can it execute twice?

What is its retry policy?

How will operators inspect failure?
```

---

## 192. Before Adding Realtime

Answer:

```text
What committed state change produces the event?

What if delivery is lost?

How does reconnect recover?

Can the event be duplicated?

Does the event contain sensitive data?
```

---

## 193. Before Adding Encryption

Answer:

```text
What threat is being mitigated?

Who owns the key?

Where is the key stored?

What happens on another device?

What happens on member removal?

How is rotation performed?

What functionality disappears?
```

---

## 194. Definition of Done

A feature is not done because the UI works.

When applicable, completion includes:

```text
migration

constraints

indexes

sqlc queries

service/domain behavior

authorization

RLS

transaction semantics

concurrency behavior

idempotency

audit

structured errors

OpenAPI

realtime/event contract

unit tests

integration tests

security tests

telemetry

failure behavior

documentation
```

---

## 195. Project Priority — Tier 0

Platform foundation must be excellent.

```text
configuration
structured logging
bounded local password/BFF session auth
sessions
organization provisioning
tenant executor
tenant migrations
team resolution
RLS
authorization
OpenAPI
durable jobs
observability
```

Do not build ten product modules on an unstable platform.

---

## 196. Project Priority — Tier 1

Core product:

```text
organizations
teams
memberships

tasks/Kanban

encrypted file vault

research artifact registry

experiment registry
provenance

resource booking

audit
```

These should receive the strongest engineering work.

---

## 197. Project Priority — Tier 2

Collaboration:

```text
chat
notifications
collaborative documents
search
realtime presence
```

---

## 198. Project Priority — Tier 3

AI:

```text
summarization
action extraction
task suggestion
research context retrieval
experiment comparison
```

AI must not delay foundational security/reliability work.

---

## 199. Stretch Objectives

Only after the core system is reliable:

```text
passkeys

full encrypted collaborative documents

E2EE chat

tamper-evident audit checkpoints

PITR

Kubernetes

dedicated HA persistence

SCIM

mobile client

advanced AI agents
```

---

## 200. Explicit Non-Goals

Do not build:

```text
custom OAuth provider

custom CRDT

custom cryptographic primitive

custom message broker

custom orchestration platform

event sourcing for everything

full CQRS

Kafka merely for architecture

service mesh

database-per-team

dozens of microservices

multi-region consensus
```

These do not increase the project's engineering quality proportionally to their cost.

---

## 201. Evaluation Demonstrations

The final demonstration should intentionally show system behavior under failure and adversarial conditions.

### Tenant schema isolation

Create two organizations.

Show their separate PostgreSQL schemas.

Attempt cross-organization access.

Expected:

```text
denied
```

---

### RLS defense

Execute a task query with no:

```sql
WHERE team_id = ...
```

Expected:

```text
only active-team rows
```

---

### Pool leakage

Alternate organizations/teams under concurrent load.

Expected:

```text
zero cross-tenant rows
```

---

### Concurrent task editing

Two clients update the same version.

Expected:

```text
one succeeds

one receives 412
```

---

### Booking race

Two clients reserve the same resource/time.

Expected:

```text
one succeeds

one rejected by database-backed invariant
```

---

### Worker crash

Commit application mutation.

Kill worker before side effect.

Restart.

Expected:

```text
durable work resumes
```

---

### Multi-node realtime

```text
Browser A → API path A

Browser B → API path B
```

Update state.

Browser B receives event through shared realtime infrastructure.

---

### Redis outage

Expected:

```text
realtime/presence degradation

persistent PostgreSQL state remains correct
```

---

### Encrypted storage

Upload confidential research file.

Inspect MinIO object.

Expected:

```text
ciphertext
```

Download and decrypt through authorized client.

---

### Team revocation

Remove team member.

Rotate team key.

Expected:

```text
removed member cannot obtain new key material
```

---

### AI authorization

Ask AI about another team's data.

Expected:

```text
retrieval layer refuses access
```

---

### Observability

Trace:

```text
HTTP
→ tenant transaction
→ DB mutation
→ durable job
→ worker
→ realtime
```

using tracing.

---

### Backup restore

Create real data.

Backup.

Destroy environment.

Restore.

Verify hashes and application state.

---

## 202. Engineering Evidence

Final evaluation/report should contain evidence, not only diagrams.

Examples:

```text
RLS isolation test results

concurrency test results

failure injection results

k6 latency distribution

p95/p99 measurements

EXPLAIN ANALYZE example

pprof analysis

restore drill

ASVS verification matrix

threat model

architecture ADRs

trace screenshots

Grafana dashboard

encrypted-object inspection
```

---

## 203. What the Team Must Be Able to Explain

Every team member should understand:

```text
why schema-per-org was chosen

how schema resolution is trusted

why SET LOCAL matters with pgxpool

why team_id RLS exists

why RLS does not replace authorization

why BIGINT and UUID both exist

how local password/BFF session authentication works

why frontend JS does not hold bearer tokens

how durable jobs survive crashes

why jobs must be idempotent

why WebSockets are not authoritative state

why PostgreSQL + MinIO need state machines

how envelope encryption works

what revocation cannot undo

how provenance is represented

how AI authorization is enforced

how the system is restored
```

These are system concepts, not module trivia.

---

## 204. Final Engineering Invariants

The following invariants define whether Synodus is correct.

### Tenancy

```text
An organization request can access only the server-resolved organization schema.

A team operation can access only rows permitted by its team context.

A reused PostgreSQL connection cannot retain the previous tenant context.
```

### Identity

```text
Clients never need internal database IDs.

Browser JavaScript never needs persistent authentication bearer tokens.
```

### Authorization

```text
Authentication alone never implies authorization.

Organization administration does not silently imply access to every team's research content.
```

### Concurrency

```text
A stale client cannot silently overwrite newer collaborative state.

Conflicting resource bookings cannot both commit.
```

### Async processing

```text
A committed important asynchronous operation is not lost because an API or worker process dies.

Duplicate job execution cannot corrupt state.
```

### Realtime

```text
Clients can recover authoritative state even if realtime delivery is lost.
```

### Privacy

```text
Confidential file plaintext is not stored by MinIO.

Private cryptographic material is not logged or stored plaintext server-side.

A feature never claims server-inaccessible encryption while secretly requiring server plaintext.
```

### Research integrity

```text
Published artifact versions are attributable.

Research output can be related to the experiment/input versions that produced it.
```

### Operations

```text
Important failures are observable.

Failed jobs are inspectable.

Tenant migrations are versioned.

The deployment has a tested restore procedure.
```

---

## 205. Final Decision Rule

Whenever the project must choose between:

```text
adding another visible feature
```

and:

```text
making an existing critical path correct under
security attacks,
concurrency,
retries,
process crashes,
partial infrastructure failure,
or restoration
```

choose the second.

Synodus should not be judged by how many screens it contains.

It should be judged by whether the team can demonstrate that:

```text
tenant boundaries survive programmer mistakes;

database state survives concurrent access;

asynchronous work survives process termination;

confidential objects remain encrypted outside authorized clients;

research outputs remain attributable and reproducible;

AI cannot bypass application authorization;

operators can observe, diagnose and restore the system.
```

That is the engineering standard for the project.

---

## 206. Concise System Definition

> **Synodus is a self-hostable, privacy-first multi-tenant research collaboration and research-operations platform combining strongly isolated organizations and teams, collaborative workflows, encrypted research storage, experiment and artifact provenance, realtime coordination and authorization-aware AI with production-oriented security, observability, failure recovery and operational tooling.**

---

## 207. Architectural Mental Model

```text
                              INTERNET
                                  │
                           HTTPS / WSS
                                  │
                           ┌──────▼──────┐
                           │    Caddy    │
                           └──────┬──────┘
                                  │
                           ┌──────▼──────┐
                           │   Go API    │
                           │    / BFF    │
                           └──┬──┬──┬────┘
                              │  │  │
            ┌─────────────────┘  │  └──────────────────┐
            │                    │                     │
            ▼                    ▼                     ▼
       PostgreSQL             Redis                  MinIO
      users/sessions             │
            │                    │
            │                    ▼
            │               Centrifugo
            │                    │
            │                    ▼
            │                 Clients
            │
            ▼
          River
            │
          Workers
            │
       ┌────┴─────┐
       ▼          ▼
    realtime      AI
                   │
                   ▼
            Python AI Service


PostgreSQL:

public
├── users
├── organizations
├── organization_memberships
├── sessions
└── control-plane data

org_A
├── teams
├── team_members
├── tasks             ─┐
├── chat               │
├── documents           │
├── files               ├── team_id + RLS
├── experiments         │
├── artifacts           │
├── resources           │
└── audit              ─┘

org_B
└── completely separate namespace


Request security:

session
   ↓
user
   ↓
organization UUID
   ↓
trusted schema
   ↓
team UUID
   ↓
trusted team BIGINT
   ↓
permission
   ↓
BEGIN
   ↓
SET LOCAL search_path
SET LOCAL app.user_id
SET LOCAL app.team_id
   ↓
sqlc
   ↓
RLS
   ↓
COMMIT


External identity:
UUID

Internal relational identity:
BIGINT


Durable truth:
PostgreSQL + MinIO

Ephemeral coordination:
Redis

Durable asynchronous work:
PostgreSQL/River

Realtime transport:
Centrifugo

Authentication uses bounded local passwords with a revocable opaque BFF session.

Team authorization:
Synodus permissions

File confidentiality:
client-side encryption

Research integrity:
immutable artifacts + provenance

Observability:
OpenTelemetry + metrics + logs + traces
```
