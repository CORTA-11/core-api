# ADR-005 — Local password authentication with BFF sessions

| Field | Value |
| --- | --- |
| Status | `accepted` |
| Date | 2026-08-26 |
| Closes | `TDR-06` |
| Applies from | M03-D01 |

## Context

The prototype already stores local Argon2id password hashes and issues bearer
JWTs. The earlier plan would have introduced Keycloak and OIDC as an M03 runtime
dependency, migrated accounts through external subject linking, and then created
an application session. That adds an identity-provider deployment and an account
linking boundary before the first release, while mutable organization/team roles
must still be resolved from Synodus databases.

Browser-held bearer tokens are difficult to revoke and must not become the
authority for tenant selection or roles. The core release needs bounded login,
revocable browser authentication, CSRF protection, and an operator-controlled
account lifecycle without public registration or recovery.

## Decision

Synodus authenticates M03 browser users locally by canonical email and password.
Passwords are normalized, policy-checked, and stored only as bounded Argon2id
hashes. Existing accounts are preserved; ambiguous canonical-email duplicates
abort migration rather than being merged. Accounts are created by an operator
CLI using an interactive secret prompt or `--password-stdin`.

Successful login creates a 256-bit opaque application session. The browser holds
the raw secret only in an HttpOnly cookie; PostgreSQL stores its SHA-256 hash and
revocation/expiry state. The BFF derives a CSRF value from the raw session token
with a dedicated HMAC secret. Roles, memberships, organization state, and tenant
identifiers are resolved from current database state on every protected path.

The M03 API has no bearer-JWT compatibility mode. Existing prototype JWTs stop
working at D06. JWT may be reconsidered only for narrowly scoped non-browser
clients, and any such token must not make mutable role or tenant claims
authoritative.

## Deferred extensions

OIDC federation, MFA, passkeys, public registration, email recovery, and
non-browser API tokens are explicitly deferred. A future OIDC implementation
must use maintained standard libraries, authorization code with PKCE where
applicable, issuer/audience/state/nonce validation, bounded discovery/JWKS
caching, and current OAuth security guidance. It must link external identity by
immutable issuer/subject and end in the same Synodus session/authorization model.

## Alternatives considered

- Keycloak/OIDC now: standards-based federation and MFA are valuable, but the
  extra deployable and account-link migration are not required for the first
  browser release.
- Keep self-contained JWT login: operationally simple, but immediate revocation,
  session inspection, rotation, and server-controlled expiry are worse.
- Store browser session state only in Redis: rejected because Redis is not
  durable authorization state in this architecture.

## Consequences

Synodus owns password-verifier hardening, operator account creation, and rehash
upgrades. PostgreSQL becomes the durable session
authority, while Redis outages affect only the M03 rate-limit envelope. The
browser never receives a durable bearer token, account roles remain fresh, and a
future IdP can replace the credential-verification step without replacing
sessions or product authorization.
