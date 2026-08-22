# Repository Guidelines

This file is the concise operational index for contributors and coding agents.
See [`PROJECT_GUIDELINES.md`](PROJECT_GUIDELINES.md) for product and architecture
decisions, [`STYLE.md`](STYLE.md) for implementation standards, and
[`CONTRIBUTION.md`](CONTRIBUTION.md) for the contribution workflow.

## Project Structure & Module Organization

`cmd/api` contains the server entry point, HTTP handlers, and middleware; `cmd/migrate` and `cmd/seed` provide database utilities. Business logic lives in `internal/service`, persistence code in `internal/repository`, and object-storage integration in `internal/minio`. SQL sources are split between `db/queries` and versioned `db/migrations/{public,tenant}`. Tests sit beside production code as `*_test.go`. Root configuration defines generation and local workflows.

## Build, Test, and Development Commands

- `cp .env.example .env`: create local configuration; never commit `.env`.
- `docker compose up -d`: start PostgreSQL, Redis, and MinIO.
- `make migrate-up-all`: apply all public and tenant migrations.
- `make seed`: load development data.
- `make run`: run the API with values from `.env`.
- `make test`: run all Go tests verbosely.
- `make fmt`, `make static`, `make sec`: format, run repository-wide Go vet and
  Staticcheck-backed linting, and run vulnerability/security checks.
- `make generate`: regenerate `internal/repository/*.sql.go` after changing SQL sources.

## Coding Style & Naming Conventions

Run `gofmt` (`make fmt`); Go indentation uses tabs. Exported identifiers use `PascalCase`, internal identifiers use `camelCase`, and package names are short and lowercase. Keep transport parsing in handlers, business invariants in services, and SQL access in repositories. Prefer bounded control flow and contextual errors. Do not manually edit generated `sqlc` files. Consult `STYLE.md` for full architecture rules.

## Testing Guidelines

Test-driven development is the default. For each behavior slice, write the
lowest-layer focused test, run it before implementation, and confirm the expected
failure; then implement, make the focused and relevant regression lanes green,
and refactor while green. Commit the test and implementation together only after
the red result has been observed and the slice passes. Record red and green
commands in the PR description; explain when documentation-only or generated-only
work has no meaningful executable red test.

Use Go's `testing` package, with `testify` and `pgxmock` where appropriate. Name
tests by observable behavior, for example
`TestTaskService_MoveTaskRejectsCrossTeamMove`. Cover failure and authorization
paths, not only successful requests. Persistence, migration, RLS, and
tenant-boundary behavior requires a real PostgreSQL test; mocks cannot prove
those properties. Run `go test -race ./...` for concurrency-sensitive changes.
No numeric coverage threshold is defined, but changed behavior must be exercised.

## Commit & Pull Request Guidelines

History mixes terse imperative subjects with Conventional Commits; follow the current `CONTRIBUTION.md` standard: `<type>(<scope>): <lowercase imperative summary>`, such as `fix(tenancy): prevent cross-team task lookup`. Use focused branches like `feat/task-comments`. Limit PRs to one logical change. Describe motivation, design, security/tenancy effects, tests, migrations, dependencies, and compatibility; link relevant issues. Include regenerated code with its source change.

## Security & Configuration

Treat organization schemas and `team_id` boundaries as invariants. Never derive schema names from untrusted client input or expose internal numeric IDs. Keep secrets out of logs and version control; add new configuration keys to `.env.example` with safe development defaults.
