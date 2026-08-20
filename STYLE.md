# Synodus Engineering & Coding Guidelines

This document defines implementation and coding standards. See
[`PROJECT_GUIDELINES.md`](PROJECT_GUIDELINES.md) for product and architecture
decisions, [`CONTRIBUTION.md`](CONTRIBUTION.md) for the contribution workflow,
and [`AGENTS.md`](AGENTS.md) for the concise repository operations index.

**Status:** Project-wide engineering standard
**Applies to:** Backend services, database code, migrations, infrastructure code, tests, supporting tools, and security-sensitive frontend code
**Primary backend:** Go
**Primary data systems:** PostgreSQL, Redis, MinIO
**Architecture:** Organization schema per tenant; `team_id` isolation inside each organization

---

## 1. Purpose

Synodus is a privacy-oriented, self-hostable collaboration platform intended to handle research data, tasks, documents, files, messages, identities, and organizational boundaries.

The purpose of this document is not merely to make the code consistent. It is to make the system:

1. Correct.
2. Secure.
3. Predictable.
4. Operable.
5. Performant.
6. Understandable.

In that order.

This guideline is inspired by TigerBeetle's TigerStyle, particularly its emphasis on safety before performance, explicit limits, simple control flow, invariants, careful naming, small dependency surfaces, and designing performance characteristics before optimization.

TigerStyle itself is designed for a mission-critical database engine written primarily in Zig. Synodus is a Go network application and therefore cannot adopt every rule literally. For example, TigerStyle prohibits dynamic allocation after initialization; Synodus instead prohibits **unbounded allocation** and requires explicit resource limits.

The governing principle is:

> Make invalid states difficult to represent, invalid operations difficult to express, and unexpected system behavior easy to detect.

---

## 2. Engineering Priorities

Every design decision should be evaluated in this order:

```text
correctness
    ↓
security
    ↓
tenant isolation
    ↓
operational reliability
    ↓
performance
    ↓
developer convenience
```

Developer convenience must never weaken the isolation or correctness model.

Performance improvements must never silently weaken correctness.

Security checks must not depend solely on convention.

Critical invariants should be enforced at multiple layers where practical:

```text
HTTP validation
        ↓
service invariant
        ↓
database constraint / RLS
```

This follows TigerStyle's principle of reinforcing important properties through multiple independent checks rather than relying on one defensive mechanism.

---

## 3. Normative Language

The words below are intentional.

* **MUST** — mandatory.
* **MUST NOT** — prohibited.
* **SHOULD** — expected unless a documented reason exists.
* **SHOULD NOT** — normally prohibited but may have justified exceptions.
* **MAY** — optional.
* **BOUND** — a finite, explicitly defined limit.
* **INVARIANT** — a property that must always hold if the program is correct.

Exceptions to a MUST require justification during code review.

---

## 4. Think Before Coding

Do not begin implementation of non-trivial functionality by immediately creating abstractions.

First determine:

```text
What invariant are we protecting?

What data owns the state?

What tenant owns the data?

What are the trust boundaries?

What can fail?

What must be bounded?

What must be atomic?

What can happen concurrently?

What must survive a restart?

What is the worst-case resource consumption?
```

For substantial changes, write the important invariants in the PR description before implementation.

Example:

```text
Invariant:
A request executing for organization A must never read from or write to
organization B's schema.

Invariant:
A team-scoped operation must never access rows belonging to another team.

Invariant:
Object metadata must never be committed unless the corresponding object
upload has reached an expected state.
```

The code should then make those statements visibly true.

---

## 5. Prefer Simple Designs

Do not optimize for the fewest lines of code.

Optimize for the smallest number of concepts required to understand the system.

Avoid:

```text
generic repositories
generic service frameworks
reflection-heavy validation
dependency injection frameworks
ORM magic
implicit transaction propagation
global mutable registries
clever middleware chains
configuration-by-side-effect
```

unless the abstraction removes more complexity than it introduces.

A small amount of duplication is preferable to the wrong abstraction.

Three similar functions do not automatically justify a generic abstraction.

Wait until the common invariant is understood.

---

## 6. Architecture Boundaries

Synodus uses explicit layers.

```text
HTTP / WebSocket
        │
        ▼
Handlers
        │
        ▼
Middleware
        │
        ▼
Services
        │
        ▼
Tenant-aware execution boundary
        │
        ├── PostgreSQL / sqlc
        ├── Redis
        ├── MinIO
        └── Realtime publisher
```

Responsibilities must remain distinct.

### 6.1 Handler

Handlers MUST:

* parse transport-level input;
* apply request-size limits;
* validate syntactic input;
* call services;
* translate service errors to HTTP responses;
* encode responses.

Handlers MUST NOT contain:

* SQL;
* transaction management;
* tenant schema manipulation;
* Redis key construction;
* MinIO implementation details;
* business authorization rules;
* complex business logic.

A handler should generally read like:

```go
func (h *Handler) createTask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		req, err := decodeCreateTaskRequest(w, r)
		if err != nil {
			writeError(w, err)
			return
		}

		task, err := h.tasks.Create(ctx, req)
		if err != nil {
			writeServiceError(ctx, w, err)
			return
		}

		writeJSON(w, http.StatusCreated, task)
	}
}
```

The important control flow is visible immediately.

---

## 7. Service Layer

Services own business behavior.

Services SHOULD express operations in domain terminology:

```go
CreateTask
MoveTask
ArchiveOrganization
RestoreOrganization
AddTeamMember
ShareFile
PublishMessage
```

rather than database terminology:

```go
InsertTask
UpdateTaskRow
SelectOrg
ExecuteQuery
```

Services MUST NOT depend on HTTP concepts such as:

```go
http.ResponseWriter
*http.Request
HTTP status codes
cookies
```

Services MUST accept `context.Context` as the first argument for operations involving I/O.

Example:

```go
func (s *TaskService) Create(
	ctx context.Context,
	input CreateTaskInput,
) (Task, error)
```

---

## 8. Repository Layer

The repository layer exists to perform persistence operations.

It MUST NOT contain business policy.

Prefer generated `sqlc` queries over handwritten database access where practical.

Do not add generic repository interfaces such as:

```go
type Repository[T any] interface {
	Create(...)
	Update(...)
	Delete(...)
	Find(...)
}
```

The database is not a generic CRUD system.

Use domain-specific queries.

Good:

```text
GetActiveOrganizationByPublicID
ListTeamTasks
RestoreOrganization
InsertSession
RevokeUserSessions
```

Bad:

```text
Get
Find
Save
UpdateEntity
Execute
Query
```

---

## 9. Tenant Isolation Is a System Invariant

Synodus uses:

```text
organization
    ↓
dedicated PostgreSQL schema

team
    ↓
team_id inside organization schema
```

This architecture is fixed.

Every developer must treat tenant isolation as a correctness property comparable to memory safety.

A cross-tenant read is not simply a bug.

It is a security failure.

---

## 10. Never Accept Tenant Schema Names From Clients

A schema name MUST NEVER originate directly from:

```text
HTTP path
HTTP header
query parameter
JSON body
WebSocket message
JWT field supplied without server validation
```

Clients work with public identifiers.

Example:

```text
organization UUID
team UUID
```

The application resolves:

```text
organization public UUID
        ↓
public.orgs
        ↓
internal organization BIGINT
        ↓
trusted schema_name
```

Only server-controlled tenant metadata may determine the PostgreSQL schema.

---

## 11. Hide Tenancy Behind an Execution Boundary

Application developers SHOULD NOT repeatedly write:

```go
tx, err := pool.Begin(ctx)
setSchema(...)
setTeam(...)
queries.WithTx(tx)
...
```

Tenancy should be centralized.

Conceptually:

```go
type TenantExecutor interface {
	WithinOrg(
		ctx context.Context,
		orgID int64,
		fn func(context.Context, *repository.Queries) error,
	) error

	WithinTeam(
		ctx context.Context,
		orgID int64,
		teamID int64,
		fn func(context.Context, *repository.Queries) error,
	) error
}
```

The executor owns:

```text
transaction creation
tenant schema selection
RLS team context
transaction-local PostgreSQL configuration
commit
rollback
logging
metrics
```

Business code should be incapable of accidentally forgetting tenant setup.

Bad:

```go
func (s *TaskService) GetTasks(
	ctx context.Context,
	schema string,
	teamID int64,
)
```

Better:

```go
func (s *TaskService) GetTasks(
	ctx context.Context,
	tenant Tenant,
) ([]Task, error)
```

or preferably obtain the resolved tenant from trusted request context.

---

## 12. PostgreSQL Tenant Context

Tenant-specific configuration MUST be transaction-local.

Never change pooled PostgreSQL session state and assume the same connection will remain associated with that request.

Conceptually:

```text
BEGIN

SET LOCAL / set_config:
    tenant schema
    team RLS context

execute queries

COMMIT
```

The transaction boundary prevents tenant context leaking into another request when the connection returns to `pgxpool`.

The application role MUST NOT be a PostgreSQL superuser.

The application role MUST NOT have `BYPASSRLS`.

Where RLS provides security isolation, tables SHOULD use `FORCE ROW LEVEL SECURITY` when appropriate so table ownership does not accidentally bypass the intended policy.

---

## 13. Team-Scoped Tables

Every team-owned table MUST contain:

```sql
team_id BIGINT NOT NULL
```

unless there is an explicit architectural reason otherwise.

Relevant tables SHOULD enforce referential integrity:

```sql
CONSTRAINT tasks_team_fk
    FOREIGN KEY (team_id)
    REFERENCES teams(id)
```

Do not rely on application code to maintain relationships PostgreSQL can enforce directly.

Tenant isolation should therefore combine:

```text
schema isolation
+
team_id
+
RLS
+
foreign keys
+
service authorization
```

These mechanisms serve different purposes and reinforce each other.

---

## 14. Internal IDs and Public IDs

Use separate internal and external identifiers.

Internal relational identifiers:

```go
int64
```

Database:

```sql
BIGINT GENERATED ... AS IDENTITY
```

External identifiers:

```go
uuid.UUID
```

Database:

```sql
UUID
```

The client MUST NOT receive internal BIGINT IDs unless there is an explicit protocol requirement.

Correct:

```text
API
UUID
 ↓
resolution boundary
 ↓
BIGINT
 ↓
internal database operations
```

This keeps external identifiers independent of internal database representation.

---

## 15. Explicit Integer Types

For persisted values, protocol values, identifiers, counters with defined limits, timestamps, sizes, or quantities, prefer explicitly sized types:

```go
int32
int64
uint32
uint64
```

Use `int` primarily where Go APIs naturally require it:

```text
slice index
slice length
loop index
standard library API
```

Do not silently convert between:

```text
int
int32
int64
uint64
```

when overflow could matter.

Conversions should occur at clear boundaries.

---

## 16. Put a Bound on Everything

TigerStyle requires bounded loops and queues because systems ultimately operate under finite resources. Synodus adopts the same principle at the application level.

There MUST be explicit limits for:

```text
HTTP request body size
file upload size
JSON payload size
WebSocket message size
WebSocket connections per user
WebSocket connections per tenant
pagination size
database result size
database connection pool
database query timeout
transaction timeout
Redis pool
Redis key TTL where applicable
Redis payload size
channel capacity
worker count
goroutine fan-out
retry count
retry duration
authentication attempts
session lifetime
token lifetime
file name length
organization name length
task title length
document size
chat message size
batch size
queue length
```

Unbounded forms are prohibited.

Bad:

```go
for _, task := range allTasksEverCreated {
	...
}
```

Better:

```text
cursor
LIMIT
bounded page size
```

---

## 17. No Unbounded Memory Consumption

Go requires dynamic memory allocation, so TigerBeetle's static-allocation rule cannot be applied literally.

The Synodus equivalent is:

> Memory consumption must be bounded by design.

Do not:

```go
io.ReadAll(r.Body)
```

for arbitrary requests.

Do not load large files entirely into memory.

Do not allow unbounded slices to accumulate WebSocket events.

Do not use an unbounded goroutine-per-item model.

Use streaming for files:

```text
HTTP body
   ↓
bounded reader
   ↓
encryption/storage pipeline
   ↓
MinIO
```

Preallocate slices when the size is known and reasonably bounded:

```go
tasks := make([]Task, 0, len(rows))
```

---

## 18. Functions Must Remain Understandable

TigerStyle uses a hard 70-line function limit to force decomposition and keep functions visible as a single reasoning unit.

Synodus adopts:

```text
Target: <= 50 lines
Normal maximum: 70 lines
```

Generated code is exempt.

A function exceeding 70 lines requires a strong reason.

Do not split functions merely to satisfy the number.

Split by responsibility.

Prefer:

```text
parent function:
    control flow

helper functions:
    computation / transformation
```

A parent function should reveal what happens.

A helper should reveal how one piece happens.

---

## 19. Push Branches Up and Loops Down

Prefer centralized control flow.

Example:

```go
func classifyResult(result Result) error {
	switch result.State {
	case StateAccepted:
		return handleAccepted(result)

	case StateRejected:
		return handleRejected(result)

	case StatePending:
		return handlePending(result)

	default:
		panic("invalid result state")
	}
}
```

rather than scattering state decisions across multiple helper functions.

Leaf functions SHOULD be as deterministic and side-effect-free as practical.

---

## 20. Avoid Deep Nesting

Prefer guard clauses for expected error handling.

Good:

```go
user, err := repo.GetUser(ctx, id)
if err != nil {
	return User{}, fmt.Errorf("get user: %w", err)
}

if user.Disabled {
	return User{}, ErrUserDisabled
}

return user, nil
```

Avoid:

```go
if err == nil {
	if !user.Disabled {
		if user.Verified {
			...
		}
	}
}
```

However, do not use guard clauses merely to produce clever negative logic.

Positive invariants should remain easy to read.

---

## 21. Avoid Compound Boolean Mazes

Bad:

```go
if user != nil &&
	user.Active &&
	!user.Deleted &&
	(role == Admin || role == Owner) &&
	team.Enabled {
	...
}
```

Prefer named decisions:

```go
if user == nil {
	return ErrUserNotFound
}

if !user.Active {
	return ErrUserInactive
}

if user.Deleted {
	return ErrUserDeleted
}

if !canManageTeam(role) {
	return ErrForbidden
}

if !team.Enabled {
	return ErrTeamDisabled
}
```

The reader should be able to identify exactly which invariant failed.

---

## 22. Recursion

Recursion SHOULD NOT be used in request-path, service, persistence, authorization, storage, or realtime code.

Use explicit iteration.

Recursion MAY be used for naturally recursive bounded data structures when:

1. the maximum depth is known;
2. the bound is enforced;
3. stack consumption is insignificant;
4. the recursive implementation is materially clearer.

---

## 23. Scope Variables Tightly

Declare variables as close as possible to their use.

Good:

```go
org, err := s.orgs.GetByPublicID(ctx, publicID)
if err != nil {
	return err
}

schema := org.SchemaName
```

Do not initialize variables at the start of a function simply because they may eventually be needed.

Shorter lifetimes reduce the number of states the reader must consider.

---

## 24. Names Carry Architecture

Names MUST reveal intent.

Good:

```text
organizationID
organizationPublicID
organizationSchema
teamID
teamPublicID

uploadSizeBytes
requestTimeout
sessionExpiresAt
pageSize
connectionLimit
```

Bad:

```text
id
pid
oid
tid
data
obj
thing
tmp
val
info
manager
helper
util
```

Short names such as:

```go
i
j
n
```

are acceptable for obvious, tightly scoped mathematical/index operations.

---

## 25. Units Must Be Visible

Do not write:

```go
timeout := 30
size := 100
```

Prefer typed durations:

```go
timeout := 30 * time.Second
```

and descriptive names:

```go
uploadSizeBytes
pageSizeMax
requestTimeout
sessionLifetime
```

For durations, prefer `time.Duration`.

Do not manually represent milliseconds as an integer inside Go application code unless a protocol requires it.

---

## 26. Avoid Boolean Parameters

This is difficult to understand:

```go
createUser(ctx, true, false)
```

Prefer an options type:

```go
type CreateUserOptions struct {
	Verified bool
	Admin    bool
}
```

or domain operations:

```go
CreateUser(...)
CreateAdmin(...)
```

depending on semantics.

Two arguments of identical primitive types that are easy to swap SHOULD use a struct.

Bad:

```go
MoveTask(ctx, 12, 13)
```

Better:

```go
MoveTask(ctx, MoveTaskInput{
	TaskID:   12,
	ColumnID: 13,
})
```

---

## 27. Interfaces Must Represent Boundaries

Do not create an interface merely because a concrete type exists.

Bad:

```go
type TaskService interface {
	Create(...)
	Get(...)
}
```

when only one implementation exists and no meaningful replacement boundary is required.

Interfaces are justified for:

```text
external storage
clock
cryptographic provider
mailer
realtime publisher
cache
test-controlled external system
```

Prefer small interfaces owned by the consumer.

Example:

```go
type ObjectStore interface {
	Put(
		ctx context.Context,
		key string,
		body io.Reader,
		size int64,
	) error
}
```

Avoid interfaces containing dozens of unrelated methods.

---

## 28. Errors: Expected vs Unexpected

Errors fall into two major categories.

### Expected operational errors

Examples:

```text
invalid request
not found
permission denied
duplicate resource
expired session
conflict
request cancellation
database unavailable
MinIO unavailable
Redis unavailable
```

These must be returned and handled.

### Programmer/invariant failures

Examples:

```text
impossible enum value
invalid state constructed internally
tenant executor invoked without resolved tenant
unreachable internal branch
```

These indicate defects.

Do not convert programmer errors into silent defaults.

---

## 29. Never Ignore Errors Accidentally

Every error MUST be:

```text
returned
handled
logged at the responsible boundary
or explicitly discarded with justification
```

Correct deliberate discard:

```go
defer func() {
	_ = tx.Rollback(ctx)
}()
```

because rollback after commit is intentionally harmless.

For anything less obvious:

```go
if err := body.Close(); err != nil {
	slog.WarnContext(ctx, "close request body", "error", err)
}
```

TigerStyle explicitly emphasizes that error paths are part of the program and must be tested rather than treated as secondary behavior.

---

## 30. Wrap Errors With Context

Use `%w`.

Good:

```go
return fmt.Errorf("create task: %w", err)
```

Good:

```go
return fmt.Errorf(
	"resolve organization %s: %w",
	publicID,
	err,
)
```

Bad:

```go
return fmt.Errorf("failed: %q", err)
```

Bad:

```go
return errors.New(err.Error())
```

The original error chain must remain available for `errors.Is` and `errors.As`.

Avoid repeatedly adding useless words such as `failed to`.

Prefer:

```text
create organization: insert organization: duplicate key
```

over:

```text
failed to create organization: failed to insert organization:
failed because duplicate key
```

---

## 31. Logging Happens at Boundaries

Do not log the same error at every layer.

Usually:

```text
repository
    return enriched error

service
    classify / return enriched error

handler / worker boundary
    log unexpected failure once
```

Expected client validation errors generally do not require `ERROR` logging.

Use:

```go
slog.ErrorContext(...)
```

for unexpected server-side failures.

Use:

```go
slog.WarnContext(...)
```

for recoverable anomalous conditions worth operator attention.

Routine invalid client input normally requires no log or, when operationally useful, structured low-severity logging.

---

## 32. Structured Logging Only

Production logs MUST be structured.

Prefer fields:

```go
slog.ErrorContext(
	ctx,
	"create task",
	"error", err,
	"organization_id", orgID,
	"team_id", teamID,
)
```

Do not construct pseudo-structured strings:

```go
slog.Error(
	fmt.Sprintf(
		"failed task org=%d team=%d err=%v",
		orgID,
		teamID,
		err,
	),
)
```

Never log:

```text
passwords
password hashes
access tokens
refresh tokens
CSRF secrets
encryption keys
plaintext encrypted documents
private research data
session cookies
authorization headers
```

---

## 33. Panic Policy

`panic` is NOT normal error handling.

Request input must never intentionally cause a panic.

Expected infrastructure failure must not cause a panic.

Acceptable uses include:

```text
invalid startup configuration
impossible internal states
violated programmer invariants
```

The HTTP recoverer exists as containment, not as control flow.

A recovered panic MUST be treated as an internal defect and logged accordingly.

---

## 34. Assertions in Go

Go has no built-in assertion mechanism.

Synodus therefore encodes invariants through:

```text
types
database constraints
validation
explicit checks
panic for truly impossible states
tests
```

Example:

```go
func stateName(state taskState) string {
	switch state {
	case taskPending:
		return "pending"
	case taskRunning:
		return "running"
	case taskDone:
		return "done"
	default:
		panic("invalid task state")
	}
}
```

Do not write custom `assert()` calls everywhere merely to imitate another language.

Use the Go mechanism that most directly expresses the invariant.

---

## 35. Validate at Trust Boundaries

Everything entering from outside the trusted process is untrusted.

Validate:

```text
HTTP bodies
path parameters
query parameters
headers
JWT claims
WebSocket messages
uploaded metadata
file names
pagination values
Redis data when assumptions matter
database values originating from legacy/untrusted data
external service responses
```

Validation inside handlers deals primarily with representation.

Business validity belongs in services.

---

## 36. Authorization Is Not Validation

These are separate:

```text
"Is team_id syntactically valid?"
```

and:

```text
"May this user access this team?"
```

Authorization MUST occur using server-resolved identity and tenant state.

Never trust a user-provided relationship such as:

```json
{
  "user_id": "...",
  "team_id": "..."
}
```

as proof that the user belongs to that team.

---

## 37. Database Constraints Are Part of the Application

Use PostgreSQL to enforce invariants it can express naturally.

Prefer:

```text
PRIMARY KEY
UNIQUE
NOT NULL
FOREIGN KEY
CHECK
RLS
transaction isolation
```

over equivalent application-only checks.

Application checks may still exist to return better errors.

Example:

```text
service checks duplicate
+
database UNIQUE constraint
```

The database constraint remains authoritative under concurrency.

---

## 38. Transactions Must Encode Atomicity

A transaction should correspond to an actual atomic business operation.

Good:

```text
create task
+
create audit record
+
update related state
```

when all must succeed together.

Do not wrap unrelated work in large transactions.

Do not perform slow external I/O while holding a PostgreSQL transaction unless the consistency model explicitly requires it.

Avoid:

```text
BEGIN
database query
HTTP request to remote service
large MinIO upload
more database work
COMMIT
```

Long transactions increase lock duration and consume pool capacity.

---

## 39. Always Bound Database Queries

Production queries MUST NOT accidentally return unbounded collections.

Prefer:

```sql
LIMIT $1
```

with a server-enforced maximum.

Do not trust the client-provided page size directly.

Example:

```go
const pageSizeMax int32 = 100

if pageSize > pageSizeMax {
	pageSize = pageSizeMax
}
```

For large datasets, prefer cursor/keyset pagination over increasingly large offsets.

---

## 40. Avoid `SELECT *`

Application SQL SHOULD specify columns explicitly.

Prefer:

```sql
SELECT
    id,
    public_id,
    title,
    status,
    created_at
FROM tasks
```

This documents the data contract and avoids silently coupling code to unrelated schema changes.

Generated migration/introspection tooling may be exempt.

---

## 41. Database Migrations

Migrations are production code.

Every migration MUST have:

```text
up migration
down migration
clear name
reviewed data implications
reviewed locking implications
```

except when a down migration is inherently unsafe, in which case that decision must be documented explicitly.

Migrations MUST NOT be rewritten after they are part of shared history.

Create a new migration instead.

Schema evolution should occur alongside the feature that needs it.

Do not create dozens of speculative tables for future functionality.

---

## 42. Redis Is Not the Source of Truth

PostgreSQL remains authoritative unless a subsystem explicitly specifies otherwise.

Redis may provide:

```text
cache
ephemeral coordination
pub/sub
rate limiting
temporary session-related state
```

Application correctness MUST survive cache loss where the cache is defined as optional.

Cache keys must be structured.

Example:

```text
synodus:v1:org:123:user:456:public_key
synodus:v1:org:123:user:456:profile
```

Do not mix unrelated values behind ambiguous keys.

For multiple related fields that require independent access, use a Redis hash or distinct typed keys according to access patterns.

---

## 43. Cache Invalidation Must Be Explicit

For every cache entry, answer:

```text
Who writes it?

Who invalidates it?

What is its TTL?

What happens after Redis restart?

Can stale data cause a security failure?

Can the system reconstruct it?
```

Authorization decisions SHOULD NOT depend on potentially stale cached state unless the consistency implications are explicitly designed.

Security-critical revocation state requires stronger treatment than ordinary performance caches.

---

## 44. Redis Pub/Sub Is Ephemeral

Redis Pub/Sub must be treated as transient delivery.

Do not design persistent correctness around the assumption that every subscriber receives every publication.

If an event must survive:

```text
process restart
subscriber disconnect
Redis restart
temporary network failure
```

it requires a durable mechanism.

Realtime updates may use Pub/Sub when clients can reconstruct authoritative state from PostgreSQL after reconnecting.

---

## 45. MinIO/Object Storage

Do not make bucket/object names arbitrary client-controlled strings.

Create object keys from server-controlled identifiers.

Conceptually:

```text
organization
/
team
/
resource UUID
/
object UUID
```

Do not expose raw storage topology unnecessarily through the public API.

File operations must define:

```text
maximum size
content metadata policy
streaming behavior
failure semantics
cleanup behavior
authorization
encryption boundary
```

---

## 46. Stream Large Files

Never buffer large uploads into a `[]byte`.

Prefer:

```go
func (
	ctx context.Context,
	reader io.Reader,
	sizeBytes int64,
) error
```

over:

```go
func (
	ctx context.Context,
	file []byte,
) error
```

when handling potentially large resources.

Streaming should remain streaming through the stack where possible.

---

## 47. Context Propagation

Every request-derived I/O operation MUST use the originating context.

```go
ctx := r.Context()
```

Propagate it through:

```text
service
database
Redis
MinIO
external APIs
```

Do not replace request contexts with:

```go
context.Background()
```

inside request-processing code.

`context.Background()` belongs primarily at application/process boundaries.

---

## 48. Timeouts Are Mandatory

External operations require finite deadlines.

Configure explicit timeouts for:

```text
HTTP server
HTTP clients
database operations
Redis
MinIO
shutdown
WebSocket writes
background jobs
```

Do not rely blindly on library defaults.

TigerStyle similarly recommends explicitly specifying important options instead of depending on defaults that may be unclear or change over time.

---

## 49. Goroutines Must Have Owners

Every goroutine must have an answer to:

```text
Who starts it?

Who stops it?

What bounds how many exist?

How is cancellation communicated?

Where do its errors go?

What happens during shutdown?
```

Forbidden:

```go
go doSomething()
```

without understanding its lifecycle.

Prefer structured ownership through:

```text
context cancellation
errgroup
bounded worker pools
server lifecycle
```

---

## 50. Never Spawn Unbounded Goroutines

Bad:

```go
for _, item := range items {
	go process(item)
}
```

when `items` can grow arbitrarily.

Use a bounded worker model.

```text
input queue
    ↓
N workers
    ↓
bounded concurrency
```

The worker limit should be a deliberate configuration value.

---

## 51. Channels Must Be Bounded by Design

A channel capacity is part of system behavior.

Do not select:

```go
make(chan Event, 10000)
```

without understanding why `10000` is appropriate.

For every queue determine:

```text
maximum depth
producer rate
consumer rate
overflow behavior
shutdown behavior
```

When the queue becomes full, define whether the system:

```text
blocks
drops
rejects
coalesces
disconnects
```

Never let this behavior emerge accidentally.

---

## 52. Synchronization Must Be Obvious

Prefer ownership over shared mutable state.

When synchronization is required, use the simplest primitive that represents the requirement.

Typical preference:

```text
immutable data
↓
single owner
↓
mutex
↓
atomic
↓
complex lock-free algorithm
```

Do not use atomics merely because they look performant.

If the correctness of an atomic operation cannot be explained precisely in review, it should not be merged.

---

## 53. WebSocket Connections

WebSockets are long-lived resources.

Each connection MUST have:

```text
maximum message size
authentication state
tenant identity
team authorization
read deadline / liveness strategy
write deadline
bounded outbound queue
cancellation mechanism
cleanup path
```

Slow clients MUST NOT be allowed to grow unbounded server memory.

Explicitly define backpressure behavior.

A client that cannot consume events sufficiently quickly may need to be disconnected and required to reconstruct state.

---

## 54. Realtime Events Are Hints, State Is Authoritative

Where practical:

```text
WebSocket event:
    "task changed"

PostgreSQL:
    authoritative task state
```

Clients should be able to recover by refetching state after:

```text
reconnect
missed event
server restart
Redis restart
network interruption
```

Avoid correctness protocols that depend on perfect WebSocket delivery unless a durable event model is intentionally implemented.

---

## 55. Security-Sensitive Code Must Be Boring

Cryptography, authentication, authorization, tenancy, sessions, CSRF protection, password handling, and key-management code should favor explicit conventional implementations.

No clever abstractions.

No hidden defaults.

No custom cryptographic primitives.

No security-by-obscurity mechanisms.

For important security flows, comments should explain the threat being prevented.

Example:

```go
// Compare the CSRF value from the request header with the value
// associated with the authenticated browser session. The cookie is sent
// automatically by the browser; requiring the header proves that the
// requester can read application-controlled client state.
```

Explain why, not merely what the next line does.

---

## 56. Secrets Must Have Narrow Lifetimes

Do not keep sensitive plaintext longer than required.

Avoid unnecessary copies of:

```text
passwords
encryption keys
plaintext documents
tokens
```

Do not place sensitive values into:

```text
logs
errors
metrics labels
URLs
panic messages
traces
```

---

## 57. Performance Starts With Architecture

Do not wait for profiling to think about obviously expensive designs.

Before implementing performance-sensitive functionality, consider:

```text
network round trips
database queries
disk/object-store operations
memory allocations
serialization
lock contention
goroutine scheduling
```

TigerStyle explicitly advocates estimating network, disk, memory, and CPU behavior during design rather than treating performance as something that begins only after profiling.

Then measure actual behavior.

Architecture decides the order of magnitude.

Profiling finds the bottlenecks inside that architecture.

---

## 58. Avoid N+1 Operations

Bad:

```text
SELECT tasks

for every task:
    SELECT creator
    SELECT comments
    SELECT attachments
```

Prefer:

```text
join
batch query
explicit preload query
```

according to the access pattern.

Count network/database round trips when reviewing service implementations.

---

## 59. Batch Where It Reduces Cost

Appropriate batching may be used for:

```text
database inserts
event publication
cache operations
notifications
background processing
```

Do not batch latency-sensitive operations merely for theoretical throughput.

Every batch needs a maximum size and maximum waiting period where applicable.

---

## 60. Optimize Only With Evidence Below the Architectural Level

After the architecture is sound:

```text
benchmark
profile
modify
benchmark again
```

For performance-sensitive code, record:

```text
median
p95
p99
maximum where useful
allocation count
bytes allocated
throughput
```

Averages alone are insufficient for latency-sensitive paths.

---

## 61. Avoid Accidental Allocation in Hot Paths

For identified hot paths:

* preallocate known-capacity slices;
* reuse buffers when ownership is clear;
* avoid unnecessary `[]byte` ↔ `string` conversions;
* avoid repeated JSON serialization;
* avoid reflection-heavy processing;
* avoid building temporary maps unnecessarily.

Do not perform these optimizations blindly throughout the entire codebase.

First establish whether the path matters.

---

## 62. Dependencies Are Liabilities Until Justified

TigerBeetle follows an unusually strict dependency policy because every dependency expands safety and supply-chain exposure. Synodus cannot reasonably use zero dependencies, but adopts the underlying principle: every dependency must earn its place.

A new dependency requires answering:

```text
What problem does it solve?

Can the standard library solve it adequately?

Is it actively maintained?

How large is its transitive dependency graph?

Does it handle sensitive data?

Can we understand the relevant code?

What is the migration cost if it disappears?

What new security surface does it introduce?
```

Do not add a package to save ten obvious lines of code.

---

## 63. Preferred Backend Toolset

Standardize around the existing toolchain.

```text
Go standard library
chi
pgx
sqlc
golang-migrate
go-redis
MinIO Go SDK
slog
established cryptographic libraries
```

Do not introduce competing libraries for the same responsibility without a strong reason.

For example, avoid simultaneously maintaining:

```text
pgx + database/sql abstraction + ORM
```

without an architectural need.

---

## 64. Generated Code Is Generated Code

Generated code MUST be visibly separated from handwritten code.

Examples:

```text
sqlc output
mocks where generation is justified
OpenAPI generation
```

Do not manually edit generated files.

Generated code may be exempt from:

```text
function length
line length
style constraints
complexity limits
```

but should still pass compilation and relevant security checks.

---

## 65. Go Formatting

All Go code MUST pass:

```bash
gofmt
```

or:

```bash
go fmt
```

No manually aligned formatting that `gofmt` destroys.

Do not debate formatting already decided by the language formatter.

---

## 66. Line Length

Go does not automatically wrap code through `gofmt`, so Synodus uses:

```text
100 columns: preferred
120 columns: normal upper bound
```

Exceptions are acceptable for inherently indivisible values such as:

```text
URLs
generated declarations
long import paths
test vectors
```

Do not damage readability merely to satisfy the column count.

---

## 67. Comments Explain Why

Bad:

```go
// Increment count.
count++
```

Good:

```go
// Count the object before publishing the event so that observers never
// see a version that has not yet been committed locally.
count++
```

Comments should explain:

```text
invariant
reason
tradeoff
security property
non-obvious algorithm
unexpected system behavior
```

Delete comments that merely translate Go into English.

TigerStyle similarly places particular emphasis on documenting the reasoning behind implementation decisions.

---

## 68. TODO Policy

A TODO is not a substitute for solving a known correctness or security problem.

Forbidden:

```go
// TODO: add tenant authorization later.
```

in merged production paths.

Acceptable:

```go
// TODO(#184): combine these two queries once sqlc supports the desired
// composite mapping. The current implementation is correct but requires
// an additional round trip.
```

Every substantial TODO SHOULD reference an issue.

Technical debt affecting:

```text
security
tenant isolation
data integrity
correctness
```

MUST be resolved before merge.

---

## 69. No Dead Code

Do not keep:

```text
commented-out implementations
unused helper functions
obsolete feature flags
unused structs
speculative abstractions
```

Git contains history.

Delete code that is no longer used.

---

## 70. Configuration Is Explicit

Production-affecting configuration must be visible and validated during startup.

Examples:

```text
database URL
Redis address
MinIO endpoint
HTTP limits
timeouts
connection pool limits
trusted origins
token lifetimes
upload size limits
```

Fail startup for invalid mandatory configuration.

Do not defer obvious configuration failure until the first request.

---

## 71. Constants Must Explain Policy

Bad:

```go
if len(name) > 255 {
```

Better:

```go
const organizationNameLengthMax = 255

if len(name) > organizationNameLengthMax {
```

Important limits should be named.

Avoid magic values, particularly for:

```text
timeouts
sizes
retries
pool limits
permissions
protocol values
```

---

## 72. Enumerations Must Be Closed

Domain states should use named types.

```go
type TaskStatus uint8

const (
	TaskStatusTodo TaskStatus = iota + 1
	TaskStatusInProgress
	TaskStatusDone
)
```

Do not let arbitrary strings spread throughout business logic.

External serialization may still expose strings if appropriate.

Translate once at the boundary.

---

## 73. Zero Values Must Be Considered Intentionally

For every important struct field, ask whether its zero value is meaningful.

Example:

```go
type TeamID int64
```

If zero is invalid, validation should make that explicit.

Do not accidentally interpret:

```text
0
""
nil
false
```

as valid domain state merely because Go initializes them automatically.

---

## 74. Data Transfer Types and Domain Types

Do not allow HTTP DTOs to become the entire application's internal model.

Prefer:

```text
CreateTaskRequest
        ↓
validated
        ↓
CreateTaskInput
        ↓
service
```

This prevents transport concerns from leaking indefinitely into business logic.

Do not create unnecessary conversion layers when the types are genuinely identical and have the same invariants.

---

## 75. Testing Strategy

Every feature should be tested at the lowest layer capable of proving the property.

Use:

```text
unit tests
integration tests
database tests
HTTP tests
security tests
race tests
fuzz tests
end-to-end tests
```

according to the invariant.

Do not try to replace database testing with mocks when the property being tested belongs to PostgreSQL.

---

## 76. Business Logic Unit Tests

Pure business logic should normally require no infrastructure.

Example:

```go
func TestCanMoveTask(t *testing.T) {
	...
}
```

Use table-driven tests when multiple input/output cases share the same structure.

Do not force every test into a table if it decreases clarity.

---

## 77. Concrete Persistence Requires a Real Database

Repository and migration behavior MUST be tested against real PostgreSQL.

Mocks cannot prove:

```text
SQL validity
foreign keys
RLS
transactions
constraints
PostgreSQL type behavior
locking behavior
migration behavior
search_path behavior
```

Integration tests should verify these directly.

---

## 78. Tenant Isolation Tests Are Mandatory

The test suite MUST intentionally attempt cross-tenant access.

Examples:

```text
org A cannot query org B schema

team A cannot read team B rows

team A cannot update team B rows

team A cannot delete team B rows

RLS remains active through repository operations

connection reuse cannot leak previous tenant context
```

These should be treated as core security tests rather than optional integration tests.

---

## 79. Test Negative Space

For every important valid operation, consider adjacent invalid cases.

Example:

```text
valid team ID
wrong team's ID
nonexistent team ID
zero team ID
deleted team
organization mismatch
unauthorized user
```

Tests should not prove only that the happy path works.

TigerStyle explicitly emphasizes testing both expected valid states and the invalid space around them.

---

## 80. Error Paths Must Be Tested

Test:

```text
database unavailable
Redis unavailable
MinIO unavailable
transaction rollback
duplicate values
context cancellation
timeouts
invalid JSON
oversized bodies
invalid UUIDs
failed authorization
WebSocket disconnect
```

A system whose success path is tested but failure paths are not tested is incomplete.

---

## 81. Race Testing

Concurrency-sensitive Go code MUST periodically run:

```bash
go test -race ./...
```

Critical concurrent components SHOULD have focused race tests.

Examples:

```text
WebSocket hub
publisher
cache wrappers
background workers
shared registries
shutdown paths
```

---

## 82. Fuzz Testing

Fuzz high-value parsers and boundary code where useful.

Candidates include:

```text
request parsers
token parsers
identifier parsing
WebSocket message decoding
permission/state transition logic
file metadata parsing
```

The objective is not high fuzz-test count.

Target state spaces where malformed input may expose unexpected assumptions.

---

## 83. Test Names Explain Behavior

Prefer:

```go
func TestTaskService_MoveTaskRejectsCrossTeamMove(t *testing.T)
```

over:

```go
func TestMoveTask2(t *testing.T)
```

A test failure should immediately identify the property that broke.

---

## 84. CI Is Part of the Definition of Done

Code MUST pass the project's automated quality gates before merge.

At minimum:

```text
formatting
build
unit tests
integration tests where configured
go vet / linting
golangci-lint
govulncheck
gosec
```

Security tooling is defense in depth.

A green scanner does not prove secure code.

A scanner warning must be understood before it is suppressed.

---

## 85. Suppressions Require Explanation

Bad:

```go
//nolint
```

Good:

```go
//nolint:gosec // G115: value was range-checked against math.MaxInt32 above.
```

A suppression must document why the tool is wrong or why the risk is accepted.

---

## 86. Build and Test in the Project Environment

Where possible, CI tools should run using controlled project tooling rather than depending on arbitrary developer-machine versions.

Docker Compose or pinned CI environments may be used for:

```text
PostgreSQL
Redis
MinIO
integration tests
security tooling
```

Reproducibility is more important than individual workstation preferences.

---

## 87. Graceful Shutdown

Server processes must define shutdown behavior.

Shutdown should:

```text
stop accepting new requests
cancel owned background work
stop consumers
close WebSocket connections appropriately
wait for bounded in-flight work
close external resources
terminate within a finite timeout
```

A process must never wait indefinitely during shutdown.

---

## 88. Startup Is a Validation Phase

Before accepting traffic, validate required dependencies and invariants where practical.

Examples:

```text
configuration parses
database pool can connect
required database state exists
cryptographic configuration is valid
MinIO configuration is syntactically valid
required limits are positive
```

Do not scatter deterministic startup errors across runtime request paths.

---

## 89. Observability Has Bounded Cardinality

Metrics labels MUST NOT contain arbitrary values such as:

```text
user UUID
organization UUID
task UUID
file name
email address
raw URL
```

Use low-cardinality dimensions.

Appropriate labels include carefully bounded categories such as:

```text
HTTP method
route pattern
status class
operation
result
```

Identifiers belong in logs/traces where appropriate, not metrics label sets.

---

## 90. Audit Events Are Different From Logs

Security/business audit history should not depend on application logs.

If Synodus needs durable auditability, create explicit audit records for operations such as:

```text
membership changes
role changes
file sharing
key changes
organization deletion/restoration
sensitive administrative actions
```

Operational logs and durable audit history serve different purposes.

---

## 91. API Design

Endpoints SHOULD operate on public resource identifiers.

API behavior should be predictable.

Prefer resources:

```text
POST   /organizations
GET    /organizations/{organization_id}
POST   /teams
GET    /tasks/{task_id}
PATCH  /tasks/{task_id}
```

over RPC-like endpoint proliferation unless the operation is genuinely an action:

```text
POST /organizations/{id}/restore
```

or equivalent project convention.

Consistency across the API is more valuable than ideological REST purity.

---

## 92. Error Responses Must Not Leak Internals

Clients should receive stable application errors.

Never expose:

```text
SQL query
schema name
database host
stack trace
internal filesystem path
MinIO credentials
raw database error
```

Production response:

```json
{
  "error": {
    "code": "task_not_found",
    "message": "task not found"
  }
}
```

Internal logs can retain diagnostic error chains.

---

## 93. Public and Internal Models Must Remain Separate

Internal fields such as:

```text
BIGINT IDs
schema_name
storage object key
password hash
cryptographic metadata not intended for clients
```

MUST NOT accidentally serialize into public responses.

Avoid directly JSON-encoding database models when they contain internal fields.

Use explicit response types where necessary.

---

## 94. Delete and Restore Semantics

Synodus uses soft deletion where domain requirements call for recoverability.

Soft deletion must have precise semantics.

Queries must make it obvious whether they include deleted resources.

Prefer names such as:

```text
GetActiveOrganization
GetOrganizationIncludingDeleted
ListActiveTeams
RestoreOrganization
```

Avoid hidden filters whose behavior cannot be inferred from the query name.

---

## 95. Cryptographic Boundaries

Encryption architecture must remain explicit.

For client-side encrypted resources:

```text
server
    stores ciphertext
    stores required metadata
    manages authorized encrypted key material as designed

server
    does not silently become the plaintext processing boundary
```

Do not add server-side features that require plaintext without recognizing that they alter the privacy model.

Privacy architecture is an invariant, not a marketing label.

---

## 96. Commit Discipline

Commits SHOULD represent coherent changes.

Commit messages should explain the engineering change rather than simply naming a file.

Good:

```text
Enforce team RLS inside tenant transactions
```

```text
Bound WebSocket outbound queues
```

```text
Stream uploads directly to object storage
```

Bad:

```text
update files
```

```text
fix
```

```text
changes
```

TigerStyle likewise treats commit history as long-lived engineering documentation rather than temporary PR metadata.

---

## 97. Pull Requests

A non-trivial PR SHOULD answer:

```text
What changes?

Why is this design correct?

What invariants are affected?

What security boundaries are affected?

What failure cases exist?

What bounds were introduced?

How was it tested?

Are migrations involved?

Are new dependencies involved?

Are there compatibility implications?
```

Do not make reviewers reverse-engineer the intention from the diff.

---

## 98. Review the Dangerous Code First

Review priority:

```text
tenant isolation
authentication
authorization
cryptography
database transactions
concurrency
resource ownership
failure handling
external input
then ordinary functionality
```

A beautiful handler does not compensate for incorrect RLS.

---

## 99. Forbidden Patterns

The following require exceptional justification or are prohibited outright:

```text
client-provided schema names

unbounded io.ReadAll on request/file data

unbounded goroutine creation

unbounded queues

unbounded database queries

business logic inside handlers

SQL inside handlers

manual tenant setup scattered through services

global mutable request state

panic for normal request errors

ignored errors

logging secrets

using Redis cache as accidental authoritative state

relying on Pub/Sub for durable delivery

long database transactions containing large external I/O

raw internal database errors returned to clients

internal BIGINT IDs exposed unnecessarily

handwritten cryptographic primitives

security-critical TODOs

blind lint/security suppressions

new dependencies without justification

generic abstractions without a real domain requirement
```

---

## 100. Synodus Adaptation of TigerStyle

The project adopts TigerStyle conceptually as follows:

| TigerStyle principle            | Synodus adaptation                                                    |
| ------------------------------- | --------------------------------------------------------------------- |
| Safety first                    | Correctness, security and tenant isolation first                      |
| Simple control flow             | Explicit Go control flow; shallow nesting                             |
| No recursion                    | No recursion in critical/application paths                            |
| Bound everything                | Bound requests, queries, queues, goroutines, retries and memory       |
| Static memory                   | No unbounded allocation; stream large data                            |
| Explicit integer widths         | Fixed-width types for persisted/protocol values                       |
| Heavy assertions                | Types, validation, constraints, tests and panic for impossible states |
| Pair assertions                 | Defense-in-depth across service/database/security boundaries          |
| 70-line functions               | 70-line normal maximum                                                |
| Think about performance upfront | Back-of-envelope resource reasoning before implementation             |
| Batch operations                | Batch where latency/consistency permit                                |
| Minimal abstractions            | Domain-specific abstractions only                                     |
| Zero dependencies               | Strict dependency budget                                              |
| Explain why                     | Comments and PRs describe reasoning                                   |
| Strict tooling                  | Standardized Go/CI/Docker toolchain                                   |
| Test invalid states             | Explicit negative/security/error-path testing                         |

TigerStyle's original formulation includes explicit bounds, assertion-heavy design, a 70-line function limit, up-front performance reasoning, naming discipline, and a highly restrictive dependency policy. The table above intentionally translates those ideas rather than adopting implementation-specific Zig rules verbatim.

---

## 101. Definition of Done

A feature is not complete merely because the happy path works.

Before merging, the author should be able to answer yes to the following.

### Correctness

* [ ] Business invariants are explicit.
* [ ] Invalid states are rejected.
* [ ] Database constraints enforce appropriate invariants.
* [ ] Atomic operations use correct transaction boundaries.
* [ ] Error paths are handled.

### Tenancy

* [ ] Organization schema is resolved server-side.
* [ ] No client controls `schema_name`.
* [ ] Team-scoped data contains correct `team_id`.
* [ ] RLS behavior is preserved.
* [ ] Cross-tenant tests exist where relevant.

### Security

* [ ] Authorization is performed independently of input validation.
* [ ] Sensitive data is not logged.
* [ ] Internal identifiers/state are not leaked.
* [ ] Request sizes are bounded.
* [ ] Security assumptions are documented.

### Resources

* [ ] Memory consumption is bounded.
* [ ] Database results are bounded.
* [ ] Goroutine creation is bounded.
* [ ] Queues/channels are bounded.
* [ ] External calls have timeouts.
* [ ] Retry behavior is bounded.

### Code

* [ ] Control flow is obvious.
* [ ] Functions normally remain below 70 lines.
* [ ] Names describe the domain.
* [ ] Errors are wrapped with `%w`.
* [ ] No accidental ignored errors exist.
* [ ] Comments explain why rather than what.
* [ ] No unnecessary abstractions were introduced.

### Persistence

* [ ] SQL is explicit.
* [ ] Migrations exist where required.
* [ ] Migration rollback implications were considered.
* [ ] Queries were tested against PostgreSQL where necessary.
* [ ] Redis is not unintentionally authoritative.
* [ ] Object-storage failure semantics are defined.

### Concurrency

* [ ] Every goroutine has an owner.
* [ ] Cancellation is defined.
* [ ] Backpressure is defined.
* [ ] Shared state synchronization is understandable.
* [ ] Race-sensitive code has been tested appropriately.

### Testing

* [ ] Happy path is tested.
* [ ] Invalid input is tested.
* [ ] Authorization failure is tested.
* [ ] Infrastructure/error paths are tested where significant.
* [ ] Tenant isolation is tested where applicable.
* [ ] CI passes.

### Operations

* [ ] Logs provide sufficient context.
* [ ] Metrics do not introduce unbounded cardinality.
* [ ] Configuration is explicit.
* [ ] Deployment does not depend on undocumented local state.
* [ ] Startup/shutdown behavior remains bounded.

---

## 102. Final Rule

When choosing between two implementations, prefer the one for which a reviewer can more easily answer:

```text
What does this do?

Why is this correct?

What can fail?

What bounds it?

Which tenant does it operate on?

Who owns this state?

How does it stop?

How would we know if it broke?
```

Synodus code should not merely work.

Its correctness should be visible from its structure.

The objective is not maximum cleverness, maximum abstraction, or minimum lines of code.

The objective is a system whose behavior remains understandable under failure, concurrency, malicious input, tenant isolation, deployment, and future maintenance.
