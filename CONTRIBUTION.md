# Contributing to Synodus

Related project documentation includes
[`PROJECT_GUIDELINES.md`](PROJECT_GUIDELINES.md) for product and architecture
decisions, [`STYLE.md`](STYLE.md) for implementation standards, and
[`AGENTS.md`](AGENTS.md) for the concise repository operations index.

This document defines the contribution workflow, commit conventions, branch naming, pull request requirements, review standards, and repository hygiene for Synodus.

The objective is to keep the repository easy to review, easy to bisect, and safe to evolve.

Synodus is a security-sensitive, multi-tenant system. Contributions affecting tenancy, authentication, authorization, persistence, cryptography, concurrency, or migrations require additional care.

---

## 1. General Principles

All contributions should follow the project engineering guidelines.

In particular:

* correctness before convenience;
* security and tenant isolation are system invariants;
* keep control flow explicit;
* avoid unnecessary abstraction;
* bound resource usage;
* keep commits small and coherent;
* write tests for behavior rather than implementation details;
* avoid mixing unrelated changes;
* document why non-obvious decisions were made.

A contribution should leave the repository in a working state.

---

## 2. Development Workflow

The normal workflow is:

```text
issue / task
    ↓
create branch
    ↓
implement small coherent changes
    ↓
run formatting and tests
    ↓
commit
    ↓
push branch
    ↓
open pull request
    ↓
review
    ↓
CI
    ↓
merge
```

Do not develop directly on `main`.

---

## 3. Branch Naming

Branch names should describe the work being performed.

Format:

```text
<type>/<short-description>
```

Examples:

```text
feat/task-comments
fix/cross-team-task-access
refactor/tenant-executor
test/rls-isolation
docs/contribution-guidelines
chore/update-go-version
perf/task-list-query
security/session-revocation
```

Preferred branch types:

```text
feat/
fix/
refactor/
test/
docs/
chore/
perf/
security/
ci/
build/
```

Use lowercase kebab-case.

Good:

```text
feat/file-sharing
fix/websocket-backpressure
refactor/tenant-executor
```

Avoid:

```text
new-feature
my-branch
teshan-work
fix_stuff
test123
final
final-v2
```

---

## 4. Keep Branches Focused

One branch should represent one logical change.

Do not combine:

```text
authentication refactor
+
new file upload feature
+
SQL formatting changes
+
dependency upgrades
```

into one branch.

Large features may contain multiple commits, but all commits should contribute toward the same logical goal.

---

## 5. Commit Philosophy

A commit is a permanent unit of project history.

Each commit should:

* represent one coherent change;
* compile whenever reasonably possible;
* pass relevant tests;
* avoid unrelated formatting changes;
* contain enough context to understand why the change exists;
* be independently reviewable where practical.

A good commit should allow another developer to understand:

```text
what changed?
why did it change?
what behavior does it affect?
```

Git history is engineering documentation.

---

## 6. Commit Message Format

Synodus uses a Conventional Commits-inspired format.

```text
<type>(<scope>): <summary>
```

The scope is optional.

Examples:

```text
feat(tasks): add task creation service
fix(tenancy): prevent cross-team task lookup
refactor(db): centralize tenant transactions
test(auth): add expired session cases
docs: add contribution guidelines
ci: run govulncheck in Go container
perf(tasks): replace offset pagination with keyset pagination
security(auth): revoke refresh token reuse
```

---

## 7. Allowed Commit Types

### `feat`

Adds externally observable functionality.

```text
feat(tasks): add task assignment
feat(chat): support message editing
feat(files): stream uploads to object storage
```

---

### `fix`

Corrects incorrect behavior.

```text
fix(tasks): reject moves into another team
fix(auth): preserve session expiry during refresh
fix(files): close upload stream on storage failure
```

---

### `refactor`

Changes internal structure without intentionally changing behavior.

```text
refactor(tenancy): introduce tenant executor
refactor(auth): separate token parsing from validation
```

Do not label behavior changes as refactors.

---

### `test`

Adds or changes tests without changing production behavior.

```text
test(tenancy): add cross-schema isolation tests
test(tasks): cover deleted team access
```

---

### `docs`

Documentation-only changes.

```text
docs: document tenant transaction model
docs(api): describe task pagination
```

---

### `perf`

Improves performance without intentionally changing functionality.

```text
perf(tasks): batch task creator lookup
perf(cache): pipeline user key retrieval
```

Performance commits should include evidence where practical.

---

### `security`

Changes primarily intended to strengthen security.

```text
security(auth): rotate refresh tokens on use
security(rls): force row level security on team tables
security(files): reject unsafe object metadata
```

Use `security` rather than hiding security-relevant work under `fix` when security is the main concern.

---

### `chore`

Repository maintenance that does not materially change application behavior.

```text
chore: update development container
chore(deps): update pgx
```

---

### `ci`

Changes CI or automation.

```text
ci: add race detector job
ci: run gosec inside project container
```

---

### `build`

Changes build tooling or dependency/build configuration.

```text
build: upgrade Go to 1.26.6
build: add integration-test target
```

---

## 8. Commit Summary Rules

The summary should:

* use imperative mood;
* begin with lowercase after the prefix;
* not end with a period;
* normally remain below approximately 72 characters;
* describe the result, not the activity.

Good:

```text
fix(auth): reject reused refresh tokens
```

Bad:

```text
fix(auth): fixed refresh tokens.
```

Bad:

```text
changes to auth
```

Bad:

```text
update files
```

Bad:

```text
working on refresh token stuff
```

Think of the message as completing:

```text
This commit will...
```

For example:

```text
This commit will prevent cross-team task access.
```

Therefore:

```text
fix(tenancy): prevent cross-team task access
```

---

## 9. Commit Scopes

Scopes should identify a meaningful subsystem.

Common Synodus scopes include:

```text
auth
tenancy
orgs
teams
tasks
chat
docs
files
storage
cache
realtime
db
migrations
api
middleware
config
observability
ci
```

Do not create overly specific scopes such as:

```text
task-service-get-task-function
```

The scope identifies the subsystem, not the exact file.

---

## 10. Commit Body

Simple commits do not require a body.

Use a body when the reason for the change is not obvious.

Format:

```text
<type>(<scope>): <summary>

<why the change was needed>

<any important implementation or compatibility details>
```

Example:

```text
refactor(tenancy): centralize tenant transactions

Tenant-specific services previously created transactions and configured
search_path independently. This made it possible for new services to omit
tenant setup accidentally.

Introduce TenantExecutor so schema and RLS context are established through
one transaction boundary before repository access.
```

The body should explain why more than what.

The diff already shows what changed.

---

## 11. Breaking Changes

Breaking API or persistence changes must be explicitly identified.

Example:

```text
feat(api)!: replace numeric pagination with cursors

BREAKING CHANGE: task listing endpoints no longer accept page and offset.
Clients must use the returned cursor.
```

Breaking changes require explicit discussion in the pull request.

Do not introduce breaking changes accidentally.

---

## 12. Issue References

Reference relevant issues in the commit body or pull request when appropriate.

Example:

```text
Closes #184
```

or:

```text
Refs #184
```

Prefer closing issues from the pull request rather than spreading issue-management metadata across unrelated commits.

---

## 13. Commit Size

Prefer small, meaningful commits.

A commit should generally contain one conceptual operation.

Good sequence:

```text
feat(tasks): add task status database field

feat(tasks): add task status service validation

feat(api): expose task status updates

test(tasks): cover invalid status transitions
```

However, do not split changes so aggressively that intermediate commits are broken or meaningless.

This is usually worse:

```text
add struct
add function
add another function
fix compile
fix typo
tests
```

Commits should represent engineering units, not keyboard activity.

---

## 14. Avoid "Fixup History"

Before requesting review, remove meaningless development commits such as:

```text
fix
fix again
oops
forgot test
lint
format
address comment
actually fix
```

Use:

```bash
git rebase -i
```

or appropriate Git tooling to clean local history before merge when project workflow permits.

Do not rewrite commits that other developers are already depending on without coordination.

---

## 15. Do Not Hide Important History

Do not squash commits merely to make the history shorter.

Preserve meaningful architectural steps when they aid future debugging.

For example:

```text
refactor(tenancy): introduce tenant executor
security(tenancy): configure team RLS in tenant executor
test(tenancy): verify connection reuse cannot leak tenant context
```

may be more useful than:

```text
implement tenant stuff
```

Commit history should help `git bisect`, debugging, and code archaeology.

---

## 16. Atomic Commits

Do not commit unrelated changes together.

Bad:

```text
feat(tasks): add comments and update dependencies
```

Better:

```text
feat(tasks): add task comments
chore(deps): update pgx
```

Avoid repository-wide formatting inside functional commits unless the formatting is required by the change.

---

## 17. Database Migration Commits

Schema changes and the code that depends on them should be coordinated carefully.

A migration commit should normally include:

```text
up migration
down migration
sqlc query changes where needed
generated sqlc code where tracked
relevant tests
```

Example:

```text
feat(tasks): add task priority
```

Do not create migrations such as:

```text
fix migration
fix migration again
```

after they have entered shared history.

Once a migration has been shared, create a new migration.

---

## 18. Migration Naming

Use the project's migration numbering convention.

Example:

```text
000012_add_task_priority.up.sql
000012_add_task_priority.down.sql
```

Names should describe the schema operation.

Good:

```text
000013_add_team_membership_index
000014_add_task_soft_delete
000015_force_rls_on_tasks
```

Bad:

```text
000013_update_db
000014_changes
000015_fix
```

---

## 19. Generated Code

Do not manually modify generated code.

Examples include:

```text
sqlc output
generated API clients
generated mocks
```

Make the source change and regenerate.

Generated code should normally be committed together with the source that produced it when the repository tracks generated outputs.

Example:

```text
feat(tasks): add task priority query
```

may contain:

```text
db/queries/tasks.sql
internal/repository/tasks.sql.go
```

This is one logical change.

---

## 20. Formatting

Before committing Go code, run:

```bash
gofmt
```

or:

```bash
go fmt ./...
```

Do not manually fight `gofmt`.

SQL and configuration files should follow existing repository style.

Avoid formatting unrelated files.

---

## 21. Tests Before Commit

Run tests appropriate to the change.

Minimum for Go application changes:

```bash
go test ./...
```

Concurrency-sensitive changes:

```bash
go test -race ./...
```

Security or persistence changes may additionally require:

```text
integration tests
PostgreSQL tests
RLS tests
MinIO tests
Redis tests
```

Do not claim a contribution is tested if only compilation was checked.

---

## 22. Recommended Local Verification

Before opening a pull request, contributors should run the project's available equivalents of:

```bash
go fmt ./...
go test ./...
go vet ./...
golangci-lint run
govulncheck ./...
gosec ./...
```

When the project provides Makefile or Docker Compose targets, prefer those standardized commands.

Use the repository's current verification targets:

```bash
make fmt
make test
make test-race
make lint
make sec
make check
```

`make check` runs formatting, module, build, generation, migration, lint, and
security checks. Run the relevant integration or isolation targets separately
when the change affects persistence or tenant boundaries.

---

## 23. Pull Request Titles

Pull request titles should normally follow the same style as commits:

```text
feat(tasks): add task comments
fix(tenancy): prevent cross-team task access
refactor(db): centralize tenant transactions
```

This keeps release history and GitHub activity consistent.

---

## 24. Pull Request Description

Every non-trivial pull request should explain:

```text
What changed?

Why was this change needed?

What invariants are affected?

How was it tested?

Does it affect tenancy?

Does it affect security?

Does it contain migrations?

Does it introduce dependencies?

Are there deployment or compatibility concerns?
```

A recommended PR structure is:

```markdown
## Summary

Describe the change.

## Motivation

Explain why it is needed.

## Design

Explain important implementation decisions.

## Security / Tenancy

Describe any relevant effects.

## Testing

List tests performed.

## Migration

Describe schema or deployment changes, if any.
```

Remove irrelevant sections rather than filling them with meaningless text.

---

## 25. Pull Request Size

Prefer reviewable pull requests.

A PR should represent one logical feature or change.

Large functionality should be decomposed into independently meaningful changes when possible.

For example:

```text
PR 1
Tenant transaction abstraction

PR 2
Migrate task service to tenant executor

PR 3
Migrate chat service to tenant executor
```

may be easier to review than a single repository-wide rewrite.

Do not split tightly coupled changes if doing so temporarily compromises correctness.

---

## 26. Draft Pull Requests

Use draft pull requests when:

* architecture is still being discussed;
* implementation is incomplete;
* early feedback would prevent wasted work;
* a large refactor needs staged review.

Do not request final review until:

```text
code compiles
relevant tests pass
temporary debug code is removed
commit history is reasonably clean
description explains the change
```

---

## 27. Review Responsibilities

Reviewers should evaluate more than style.

Review in roughly this order:

```text
correctness
security
tenant isolation
data integrity
failure handling
resource bounds
concurrency
API compatibility
performance
maintainability
style
```

Do not spend the majority of review effort discussing naming while missing an authorization failure.

---

## 28. Author Responsibilities

The author is responsible for proving the change is safe.

Do not rely on the reviewer to discover basic defects.

Before requesting review:

* read your own diff;
* inspect changed SQL;
* inspect generated code changes;
* remove debug logging;
* remove commented-out code;
* ensure no credentials or secrets were introduced;
* run relevant tests;
* verify error paths;
* verify tenant behavior.

---

## 29. Responding to Review Comments

Review comments should be resolved through one of:

```text
change the code
explain why the current design is correct
agree on a follow-up issue
```

Do not silently mark unresolved technical concerns as resolved.

When a review comment causes an important architectural change, update the PR description.

---

## 30. Review Fix Commits

During active review, temporary commits such as:

```text
fix review comments
```

may be acceptable locally or on the PR branch depending on team workflow.

Before final merge, meaningful history should be preserved and meaningless fixup commits should be squashed where appropriate.

Prefer:

```text
fix(tenancy): reset RLS context on transaction failure
```

over:

```text
address review
```

if the change itself is significant.

---

## 31. Merge Strategy

Preferred merge behavior should preserve useful commit history while preventing noisy development history.

If commits are already well-structured:

```text
rebase and merge
```

is generally suitable.

If the PR contains many temporary fixup commits:

```text
squash and merge
```

may be preferable.

Do not blindly squash architectural history that would be useful for debugging.

The repository should ultimately maintain a mostly linear and meaningful history.

---

## 32. Direct Commits to `main`

Direct commits to `main` should be prohibited except for emergency repository administration if explicitly allowed by project maintainers.

Normal changes require:

```text
branch
+
pull request
+
CI
+
review
```

Branch protection should enforce this where practical.

---

## 33. Security-Sensitive Contributions

Changes involving any of the following require additional review:

```text
authentication
authorization
sessions
JWT
CSRF
password handling
cryptography
tenant isolation
RLS
schema selection
key management
file encryption
security headers
permission changes
```

The PR description must explicitly state the security invariant.

Example:

```text
Security invariant:

A team member can only retrieve task rows for the team selected in the
server-resolved tenant context.
```

Security changes should include negative tests.

---

## 34. Tenant-Sensitive Contributions

Any code accessing tenant data must answer:

```text
How is the organization resolved?

Who determines the schema?

How is team_id established?

Does RLS apply?

Can connection pooling leak tenant state?

Can a public UUID resolve to another tenant's internal ID?

Are cross-tenant tests present?
```

Never accept `schema_name` directly from a client.

Do not bypass the tenant executor for convenience.

---

## 35. Dependency Changes

Do not add dependencies casually.

A PR introducing a dependency should explain:

```text
what problem it solves
why the standard library is insufficient
why the selected library was chosen
security implications
maintenance implications
```

Avoid dependencies for trivial helpers.

Dependency updates should generally be isolated from unrelated feature work.

Example:

```text
chore(deps): update pgx to v5.x.x
```

---

## 36. Vulnerability Fixes

Security dependency upgrades should clearly identify the reason.

Example:

```text
security(deps): update Go to fix TLS vulnerability
```

The PR should include:

```text
affected component
vulnerability identifier
previous version
fixed version
verification performed
```

Do not suppress vulnerability findings without understanding them.

---

## 37. Logging Changes

When adding logs:

* use `slog`;
* use structured fields;
* avoid sensitive information;
* do not log expected validation failures as server errors;
* avoid duplicate logging at multiple layers.

Good:

```go
slog.ErrorContext(
	ctx,
	"create task",
	"error", err,
	"team_id", teamID,
)
```

Do not log:

```text
password
password hash
JWT
refresh token
CSRF token
encryption key
private document content
authorization header
```

---

## 38. Error Handling Changes

Errors should be wrapped using `%w`.

Example:

```go
return fmt.Errorf("create task: %w", err)
```

Do not destroy error identity:

```go
return errors.New(err.Error())
```

Expected business errors should use stable domain errors where appropriate.

Unexpected infrastructure failures should preserve their error chains.

---

## 39. API Changes

API changes should include:

```text
handler changes
request/response model changes
validation
service behavior
authorization
tests
documentation where applicable
```

Breaking API changes must be called out explicitly.

Do not expose internal BIGINT identifiers unless intentionally required.

Public APIs should use public UUID identifiers.

---

## 40. Database Changes

Database contributions should consider:

```text
constraints
foreign keys
indexes
locking
RLS
migration safety
rollback
query plans
tenant boundaries
```

Do not rely on application code to enforce invariants PostgreSQL can enforce safely.

---

## 41. Performance Changes

Performance commits should include evidence.

Example PR information:

```text
Before:
median: 240 µs
p95: 610 µs
p99: 1.2 ms

After:
median: 170 µs
p95: 390 µs
p99: 730 µs
```

Describe:

```text
benchmark setup
dataset size
number of iterations
relevant hardware/environment
```

Do not claim performance improvement from code appearance alone.

---

## 42. Concurrency Changes

Concurrency-related PRs must describe:

```text
goroutine ownership
cancellation
queue bounds
backpressure
shutdown
shared state
synchronization primitive
```

Run race testing where relevant.

Avoid unbounded goroutine creation.

---

## 43. File Upload Changes

Changes to file handling should verify:

```text
maximum upload size
streaming behavior
authorization
tenant ownership
MinIO object naming
failure cleanup
encryption behavior
content metadata handling
```

Do not replace streaming with whole-file buffering without a documented reason.

---

## 44. Configuration Changes

New configuration values must:

* have clear names;
* have documented units;
* be validated at startup;
* define sensible secure defaults where possible;
* avoid embedding secrets in source control.

Prefer:

```text
HTTP_REQUEST_TIMEOUT
MAX_UPLOAD_SIZE_BYTES
DB_MAX_CONNECTIONS
```

over ambiguous names such as:

```text
TIMEOUT
LIMIT
MAX
```

---

## 45. Environment Files

Never commit real secrets in:

```text
.env
docker-compose.yaml
test configuration
shell history
example files
```

Example configuration should use placeholders:

```text
DATABASE_URL=postgres://user:password@localhost:5432/synodus
```

not real credentials.

`.env.example` may document required variables without secrets.

---

## 46. Repository Hygiene

Do not commit:

```text
IDE configuration unless intentionally shared
temporary binaries
coverage output
profiling output
database dumps
local environment files
secrets
debug logs
editor swap files
OS metadata
```

Update `.gitignore` where needed.

---

## 47. Dead Code

Do not leave commented-out implementations.

Bad:

```go
// old implementation
// func CreateTask(...) {
//     ...
// }
```

Delete it.

Git already stores the history.

Do not merge unused experimental abstractions.

---

## 48. TODOs

TODOs should reference an issue when representing real future work.

Good:

```go
// TODO(#241): batch notification inserts once the notification schema is
// finalized.
```

Do not merge:

```go
// TODO: fix auth
```

Security, correctness, tenancy, and data-integrity TODOs must generally be resolved before merge.

---

## 49. Commit Examples

Feature:

```text
feat(tasks): add task assignment

Add assignee_id to tenant task records and expose assignment through the
task service.

Membership is validated before updating the task so users cannot assign
tasks to members outside the selected team.
```

Bug fix:

```text
fix(tenancy): reject task access across teams

Task lookup previously resolved the task by public UUID before applying
the team restriction.

Perform the lookup through the team-scoped tenant executor so RLS and
team_id both restrict the query.
```

Refactor:

```text
refactor(db): introduce tenant transaction executor

Centralize transaction creation, search_path configuration, team RLS
context, rollback, and commit handling.

This removes repeated tenant setup from individual services.
```

Security:

```text
security(auth): rotate refresh tokens on use

Invalidate the previous refresh token when issuing a replacement so a
stolen token cannot be replayed after legitimate use.
```

Performance:

```text
perf(tasks): use keyset pagination for task listing

Replace large OFFSET scans with created_at and id cursor pagination.

This reduces query latency for large task tables without changing the
default page size.
```

Migration:

```text
feat(tasks): add soft deletion

Add deleted_at to task records and update active task queries to exclude
deleted rows.

The migration preserves existing task records as active.
```

---

## 50. Bad Commit Examples

Avoid:

```text
update
```

```text
fix
```

```text
new changes
```

```text
commit before sleep
```

```text
final
```

```text
final final
```

```text
fixed bugs
```

```text
updated task.go
```

```text
refactor: changes
```

```text
feat: lots of stuff
```

These provide little value to future maintainers.

---

## 51. Before Opening a Pull Request

* [ ] The branch contains one coherent change.
* [ ] The diff has been reviewed by the author.
* [ ] No debug code remains.
* [ ] No secrets are present.
* [ ] Go code is formatted.
* [ ] Relevant tests pass.
* [ ] Integration tests were run where required.
* [ ] Race testing was performed for concurrency-sensitive work.
* [ ] Database migrations were reviewed.
* [ ] Tenant isolation was considered.
* [ ] Security implications were considered.
* [ ] New resource usage is bounded.
* [ ] Errors are handled correctly.
* [ ] Commit messages follow project conventions.
* [ ] Temporary fixup commits have been cleaned where appropriate.
* [ ] PR description explains the change and its reasoning.

---

## 52. Before Approving a Pull Request

Reviewers should verify:

* [ ] The design is understandable.
* [ ] Important invariants are explicit.
* [ ] Tenant isolation remains intact.
* [ ] Authorization is performed correctly.
* [ ] Database constraints match application assumptions.
* [ ] Error and rollback paths are correct.
* [ ] Resource usage is bounded.
* [ ] Concurrency ownership is clear.
* [ ] Tests cover important negative paths.
* [ ] Security-sensitive data is not exposed.
* [ ] Migrations are safe.
* [ ] New dependencies are justified.
* [ ] The commit history remains useful.

---

## 53. Core Rule

Every contribution should make the repository easier, not harder, to reason about.

Before committing, ask:

```text
Is this one coherent change?

Can another developer understand why it exists?

Does the commit preserve correctness?

Does it preserve tenant isolation?

Are failure cases handled?

Are resource limits explicit?

Could this commit be safely reverted?

Would this history help someone debug the system a year from now?
```

If the answer to those questions is clear, the contribution is probably in good shape.
