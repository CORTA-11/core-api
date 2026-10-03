# Synodus Core API

Go HTTP API for authentication, organizations, teams, tasks, chat, documents,
and files. PostgreSQL holds identities and the organization registry in the
public schema, with team data in per-organization schemas. Redis handles shared
rate limits and realtime event delivery; MinIO stores file bytes.

For the complete local stack with generated credentials and published images,
use the [infra installer](https://github.com/CORTA-11/infra#local-setup).

## Source development

Requires Go matching `go.mod`, Make, and running PostgreSQL, Redis, and MinIO.
Copy the local configuration once:

```bash
cp .env.example .env
cp -R dev_secrets .local_secrets
```

Configure dependencies using `.env` and `.local_secrets`, then initialize them:

```bash
make bootstrap-db   # public migrations and database role passwords
make bootstrap      # storage bucket
```

Run these in separate terminals:

```bash
make provisioner
make run
```

The API listens on <http://localhost:8080>. Check readiness with
`curl -i http://localhost:8080/health/ready` (HTTP 204). Source Compose configuration
is in `docker-compose.yaml`; the AI and realtime services live in sibling repos.
For direct API startup, run ai-service on port 8085.

Development secrets are local-only. Runtime, migration, provisioning, and admin
credentials are separate. Keep JWT and collaboration secrets aligned with
socket-server, and share any `AI_SERVICE_TOKEN` with ai-service. Configuration
options are listed in [`.env.example`](.env.example).

## Database operations

```bash
make migrate-up-all       # later public migrations
make migrate-status
make bootstrap-db         # also reapplies database role passwords
# Provisioner commands use PROVISIONING_DATABASE_URL:
go run ./cmd/provisioner status --all
go run ./cmd/provisioner retry --organization ORGANIZATION_UUID
```

Export `PROVISIONING_DATABASE_URL` when running provisioner commands directly;
`make provisioner` supplies it from local secret files. Organization creation
queues provisioning; tenant routes become available once provisioning completes.
Before deploying a new tenant migration set, stop the old provisioner, apply
public migrations, and require `status --all` to report every tenant current.

## Checks and references

```bash
make test-unit
make contract-check
make test-integration     # local dependencies required
make check
make generate             # regenerate sqlc after query/schema changes
```

- [OpenAPI contract](api/openapi.yaml): routes, payloads, and response statuses.

Chat writes and document state remain authoritative in this service and
PostgreSQL. The realtime services authenticate tickets and deliver live updates;
Redis is not a document backup.
