# M03-D01 — Local password authentication

| Field | Value |
| --- | --- |
| Status | `complete on prepared branch` |
| Branch | `security/m03-d01-password-authentication` |
| PR title | `security(auth): harden local password authentication` |
| Predecessor | M03 planning PR |
| Dependencies | M02 public repository and real-PostgreSQL fixtures |
| Merge gate | `make check`, unit, integration, race, and generate checks |

## Outcome and security invariants

Existing users authenticate through a bounded local credential verifier that
does not mint a JWT. Email lookup is canonical and case-insensitive. Password
policy follows [NIST SP 800-63B](https://pages.nist.gov/800-63-4/sp800-63b.html),
[OWASP authentication guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html),
and [OWASP password-storage guidance](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html).

- Every verifier path performs exactly one bounded Argon2id operation, including
  unknown, deleted, and malformed-account paths through a process dummy hash.
- Clients receive one `invalid credentials` classification for unknown email,
  wrong password, deleted account, and unusable stored hash.
- Encoded memory, iteration, parallelism, salt, and output sizes are rejected
  before any attacker-controlled allocation or Argon2 work.
- A process-wide weighted semaphore bounds concurrent hashes. Waiting observes
  context cancellation and a configured deadline.
- Password plaintext never reaches command arguments, logs, errors, metrics,
  database query diagnostics, or audit metadata.

## Current state and deficiencies

`public.users.email` has only a case-sensitive unique constraint. Public user
create/update endpoints accept unrestricted passwords. `userService.Login`
distinguishes repository work internally and mints a 24-hour HS256 JWT. Unknown
accounts skip Argon2. The decoder accepts hash-provided memory/time/parallelism
without safe maxima before calling `argon2.IDKey`, so a hostile or corrupt hash
can exhaust resources. There is no hash-concurrency limit, rehash path, or
operator account command. Development seeds reuse an undocumented short
password hash.

## Scope

In scope:

- Add `email_canonical` to `public.users`; preserve `email` for display/audit.
- Canonicalize valid UTF-8 email as trimmed NFC followed by Unicode case-folding;
  reject empty/control-containing values and values over 254 UTF-8 bytes.
- Preflight all existing rows in the migration. If two active or deleted users
  share a canonical value, abort with only row public IDs in diagnostics. Do not
  select a winner, merge memberships, or rewrite accounts ambiguously.
- Backfill canonical values, make the column non-null, and enforce a unique
  constraint used by every lookup/create/change path.
- Define password input as NFC-normalized Unicode, 15–128 code points inclusive
  and at most 1024 UTF-8 bytes. Permit spaces and Unicode; reject control/NUL
  characters. Apply no composition rule or periodic rotation.
- Retain Argon2id. Centralize current target parameters and hard verification
  ceilings; validate encoded string/segment sizes and decimal fields before
  base64 decode or memory allocation.
- Return `CredentialPrincipal{UserPublicID}` on success, never email, roles,
  organization/team IDs, or a token. Rehash an accepted outdated hash using a
  compare-and-swap update so concurrent successful logins are harmless.
- Add `cmd/admin user create` with a TTY prompt by default and
  `--password-stdin` for automation. Reject password flags/positionals, refuse a
  terminal on stdin mode, disable terminal echo, and clear transient byte slices
  where practical.
- Replace seed hashes with a documented policy-compliant development password
  referenced without placing it in production configuration.

Deferred: session issuance, browser handlers, CSRF, public registration, email
verification/recovery, password reset, MFA/passkeys, OIDC, and API tokens.

## Interfaces, persistence, and compatibility

`internal/identity` owns `EmailCanonicalizer`, `PasswordPolicy`,
`PasswordHasher`, and `CredentialVerifier`. Verification accepts a context and
bounded `email`/`password` values and returns either a public user ID or the
single sentinel `ErrInvalidCredentials`. Policy errors are available only to
operator creation and the authenticated password-change flow in D02.

The hash parser accepts only `$argon2id$v=19$m=...,t=...,p=...$salt$hash` with
canonical decimal fields. Target parameters begin at the repository's current
64 MiB, three iterations, parallelism four, 16-byte salt, and 32-byte output;
verification ceilings are 256 MiB, ten iterations, parallelism 16, 32-byte salt,
and 64-byte output. Parameter changes require tests and review. The semaphore
default is `max(1, min(4, GOMAXPROCS))`, configurable only within `1..16`.

The migration upgrades existing accounts in place. A temporary legacy adapter
may keep the unversioned JWT login compiling until D06, but it is frozen, calls
the new verifier, and is not used by v1 or extended with new behavior. D06
deletes that adapter and all JWT code/configuration.

## Test-first matrix

| Initial failing test/check | Expected red result | Passing criterion |
| --- | --- | --- |
| canonical-email migration test | `Alice@Example` and `alice@example` can coexist or migration guesses | upgrade aborts before mutation; a nonambiguous fleet backfills uniquely |
| canonical lookup/create test | case/normalization variant misses or duplicates | all write/read paths use one canonical value and DB uniqueness wins races |
| password-policy table | short, huge, or control-containing values hash successfully | exact character/byte bounds reject; spaces/Unicode pass |
| hostile encoded-hash test/fuzz | oversized parameters reach decode/Argon2 | malformed and over-limit inputs fail before allocation/work and never panic |
| verifier parity test | unknown email skips hash or exposes a distinct error | one bounded hash and one public error class on every invalid path |
| concurrency/cancellation test | unbounded Argon2 goroutines or stuck wait | semaphore cap holds and canceled/deadline requests leave no permit leak |
| rehash race test | old parameters remain or concurrent updates corrupt | successful login upgrades once via CAS; both valid logins remain safe |
| operator CLI test | password accepted in argv/log/error | only prompt/stdin accepted; failures redact the secret |
| seed integration test | demo secret violates policy or seed is non-idempotent | policy-compliant login succeeds after two seed runs |

Migration, duplicate, race, and seed behavior uses real PostgreSQL. Hash/parser
and CLI branches use unit/fuzz tests without printing candidate secrets.

## Ordered implementation

1. Add failing canonicalization and real-upgrade collision tests.
2. Add the public migration/query changes and regenerate; prove safe backfill,
   uniqueness, and rollback behavior.
3. Add password-policy tests for normalization and exact resource boundaries.
4. Add hostile-hash/parser tests, then implement bounded Argon2id parsing,
   hashing, semaphore admission, dummy verification, and rehash detection.
5. Add credential-verifier repository/error tests and remove JWT issuance from
   the new authentication path.
6. Add CLI red tests, then implement interactive and stdin account creation.
7. Update seed data, run upgrade/empty migrations and regressions, and record
   red/green evidence.

## Atomic green commits

1. `security(auth): enforce canonical email identity`
2. `security(auth): enforce bounded password policy`
3. `security(auth): bound argon2 hash parsing`
4. `security(auth): limit argon2 work admission`
5. `security(auth): verify credentials uniformly`
6. `security(auth): upgrade legacy credential hashes`
7. `feat(admin): add secure local account creation`
8. `test(seed): use policy-compliant development credentials`
9. `security(auth): document bounded hash conversions`
10. `docs(plan): record m03-d01 implementation`

## Verification and acceptance

Run:

```bash
make generate-check
make test-unit
make test-race
make test-integration
make check
git diff --check
```

- [x] Existing unique accounts survive upgrade with stable public IDs/hashes.
- [x] Ambiguous canonical duplicates abort without partial account changes.
- [x] Password length, Unicode, byte, and control policy is exact.
- [x] Unknown/wrong/deleted/malformed cases share one response and one hash-work class.
- [x] Argon2 parsing and concurrency remain inside declared bounds.
- [x] Successful verification rehashes outdated parameters safely.
- [x] Operator and seed workflows expose no secret and public registration is absent from v1.
- [x] The prepared branch records red and green evidence; merged PR evidence is recorded.

## Rollout, rollback, and operations

Rehearse fresh and `56d0a6d` upgrades. Before applying, run the same canonical
collision query against a backup and resolve duplicates through an explicit
operator decision; never patch the migration to merge them. Deploy the migration
before code that requires `email_canonical`. Monitor bounded verifier saturation
without account labels.

Rollback application code normally while the additive canonical column remains.
The down migration must refuse if reverting would permit ambiguity relied on by
new code. Never roll back by weakening password/hash bounds or exposing hashes.

## Handoff to D02

Provide the verifier/principal interface, canonical-email rules, dummy-hash
lifecycle, semaphore/timeout settings, target-versus-accepted Argon2 parameters,
rehash CAS query, operator command, seed credential reference, and migration
evidence. D02 creates sessions only after this verifier succeeds.

## Implementation record

**Pull request:** [#30](https://github.com/CORTA-11/core-api/pull/30)

**Merge commit:** `08bf473`

**Implementation commits:** `8f26827` canonical email; `1897bcc` password
policy and architecture cleanup; `1d2217c` bounded parser; `cdbfd1a`
hash admission; `c243906` uniform verifier and compatibility wiring; `51197c0`
CAS rehash; `de56029` admin CLI; `1c2bf1c` seeds; `15d86fc` static-security
audit annotations; this documentation commit.

**Observed red evidence:** focused `go test` runs for `./internal/identity`
failed on the expected missing `EmailCanonicalizer`, `PasswordPolicy`, bounded
Argon2 parser, `HashConfig`/hasher, credential-verifier, and CAS symbols. The
focused `go test ./cmd/admin` run failed on the expected missing CLI runner and
safe error sentinels. The first PostgreSQL migration run also exposed the legacy
seed's obsolete `ON CONFLICT (email)` target before the canonical conflict target
was applied.

**Green evidence:** `make generate-check`, `make test-unit`, `make test-race`,
`make test-integration`, `make test-isolation`, `make check`, `make secrets`, and
`git diff --check` passed on the prepared branch. PostgreSQL 18 tests cover fresh
and version-5 upgrades, collision rollback, generated canonical uniqueness,
normalization-state preservation, CAS races, the real admin CLI, and twice-applied
seeds.
