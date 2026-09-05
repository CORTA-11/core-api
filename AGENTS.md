# Repository Guidelines

These instructions apply to the entire `core-api` repository.

## Permanent repository memory

- Treat this file as the repository's permanent memory. When work reveals
  stable, generally useful knowledge that future agents should retain, add it
  here as part of the same change.
- Record only verified, durable guidance. Do not add temporary task state,
  speculative conclusions, credentials, secrets, or personal data.

## Architecture

- This is a Go service. Executables live under `cmd/`; reusable application code
  lives under `internal/`.
- Keep HTTP transport concerns in the API/HTTP packages and business rules in
  `internal/service`. Database access belongs in `internal/repository`.
- The public database holds identities and the organization registry. Tenant
  data lives in per-organization schemas. Preserve that boundary in code,
  queries, migrations, tests, and operational commands.
- Authorization must be enforced server-side. Do not infer access from a UI
  state or grant organization administrators implicit access to team content.
- `api/openapi.yaml` is the HTTP contract. Update it with any externally visible
  route, payload, status, or error change.
- Realtime chat writes remain authoritative in this service; Redis publication
  is fan-out for `socket-server`, not durable storage.

## Database changes

- Public and tenant migrations are separate in `db/migrations/public` and
  `db/migrations/tenant`. Add paired, sequential `.up.sql` and `.down.sql` files
  to the correct set; never rewrite a migration that may already be deployed.
- SQL used by the application belongs in `db/queries`. Generated `sqlc` output
  lives under `internal/repository`; change the query or schema source and run
  `make generate` instead of hand-editing generated files.
- Tenant queries must preserve row-level security and explicit tenant context.
  Include isolation coverage when changing tenant resolution, membership, or
  authorization behavior.
- Keep runtime, migration, provisioning, and bootstrap database privileges
  separate. Do not make runtime credentials capable of schema administration.

## Development workflow

- Use Go's standard formatting and idioms. Run `make fmt` after editing Go.
- Prefer small packages and explicit dependencies. Return contextual errors and
  avoid logging credentials, session values, CSRF tokens, or tenant secrets.
- Add focused tests beside the affected package. Cross-database and HTTP flows
  belong in `internal/integration`.
- Useful commands:
  - `make test-unit` — unit/package test suite.
  - `make contract-check` — OpenAPI and HTTP contract checks.
  - `make test-integration` — integration suite; requires local dependencies.
  - `make generate-check` — verify generated `sqlc` code is current.
  - `make check` — formatting, build, generation, migration, query, static, and
    security checks.
- For a narrow change, run the relevant package tests first. Before handing off
  a broad change, run `make check` and the affected integration suite when its
  dependencies are available. Report checks that could not run.

## Local configuration and secrets

- Copy `.env.example` to `.env` and `dev_secrets` to `.local_secrets` for local
  development. Never commit `.env`, `.local_secrets`, cookies, tokens, real
  credentials, or non-development connection strings. Placeholder local URLs
  in tracked examples are allowed.
- Use the Make targets documented in `README.md` for database bootstrap,
  migration, seeding, and tenant provisioning. Do not bypass the least-privilege
  credential split with an administrator connection.
- Keep shared values such as `JWT_SECRET` and `REDIS_CHAT_CHANNEL` compatible
  with `socket-server` and keep exposed routes compatible with `infra`.
