# core-api

## Development

1. Copy the non-secret environment template and development secret templates:

   ```bash
   cp .env.example .env
   cp -R dev_secrets .local_secrets
   ```

   `dev_secrets` contains development-only values and is safe to use only for a
   local stack. `.local_secrets` is ignored by Git and is the active secret
   directory used by Make, Docker Compose, and direct application startup.
   Replace its values when needed; never use the development templates in a
   deployed environment.

   The required files are `db_admin_user.txt`, `db_admin_password.txt`,
   `db_runtime_password.txt`, `db_migrator_password.txt`,
   `db_provisioner_password.txt`, `minio_root_user.txt`,
   `minio_root_password.txt`, `minio_access_key`, `minio_secret_key.txt`,
   `redis_limit_secret.txt`, `redis_invitation_binding_secret.txt`, and
   `csrf_secret.txt`.

2. Start Postgres, Redis, and MinIO without starting the API yet:

   ```bash
   docker compose up -d postgres redis minio
   ```

3. Bootstrap the database roles and apply public migrations:

   ```bash
   make bootstrap-db
   ```

   This one-time/recovery command uses the admin secret files as administrator
   credentials, applies the public role migration, and assigns the three
   operational role passwords. Normal migrations, provisioning, and API traffic
   use the separated migrator, provisioner, and runtime credentials afterward.

4. Create the configured MinIO bucket and optionally seed development data.
   `make seed` is idempotent, so it is safe to rerun:

   ```bash
   make bootstrap
   make seed
   ```

5. Start the tenant provisioner in its own terminal. It is a long-running
   process and should remain running. Wait for `status --all` to report each
   seeded organization as `"current":true` before calling tenant routes:

   ```bash
   make provisioner
   # In another terminal:
   go run ./cmd/provisioner status --all
   ```

6. Start the API in another terminal:

   ```bash
   make run
   ```

   To run the API in Docker instead, build and start its Compose service after
   completing the database and MinIO bootstrap steps above:

   ```bash
   docker compose up --build -d api
   docker compose ps
   ```

   For a fresh database, start dependencies and perform bootstrap first:

   ```bash
   docker compose up -d postgres redis minio
   make bootstrap-db
   make bootstrap
   docker compose up --build -d api
   ```

   The API is available on `http://localhost:8080`. Compose also starts the
   long-running tenant provisioner, mounts a separate least-privilege database
   secret into each service, and waits for infrastructure dependencies to
   become healthy.

   Verify startup with `curl -i http://localhost:8080/health/ready`; a ready
   development stack returns HTTP 204.

7. Start the realtime services if chat or collaborative Documents are needed:

   ```bash
   cd ../socket-server
   cp -n .env.example .env
   make run
   ```

   In another terminal:

   ```bash
   cd ../socket-server
   npm --prefix collaboration ci
   make collaboration-build
   make collaboration-run
   ```

   Direct startup requires Node.js 22+ and `npm --prefix collaboration ci`
   once before `make collaboration-run`. Docker Compose can build and run both
   realtime processes with the API:

   ```bash
   docker compose up --build -d api socket-server collaboration-server
   docker compose ps
   curl -i http://localhost:8080/health/ready
   curl -sS http://localhost:8081/health
   curl -sS http://localhost:8082/health
   ```

   `api`, `socket-server`, and `collaboration-server` use the private Compose
   network. The Document process reaches core-api at `http://api:8080`; only
   browser REST and WebSocket traffic should be exposed through Envoy. Set the
   same `JWT_SECRET`, `COLLABORATION_SERVICE_SECRET`, and allowed browser
   origins in every process.

For later public migrations use `make migrate-up-all`; do not rerun them with
runtime credentials. `make bootstrap-db` is also the recovery command when an
existing development `.env` receives new role passwords. If a password contains
URL-reserved characters, set URL-encoded `BOOTSTRAP_DATABASE_URL`,
`DATABASE_URL`, `MIGRATION_DATABASE_URL`, and `PROVISIONING_DATABASE_URL`
explicitly instead of relying on the component-derived development URLs.

### Development seed data

All seeded users use the development-only password `synodus-demo-password`.
Each account stores a distinct target-parameter hash.

| User | Public user ID | Organization memberships |
| --- | --- | --- |
| `admin@aratuwa.edu` | `0d5a4f4e-8d3b-4f17-9a79-4c38e29a6d11` | University of Aratuwa, MedSync, Pied Piper |
| `leader@aratuwa.edu` | `48b38b47-36a8-4758-9858-c28c222d2c2e` | University of Aratuwa, MedSync |
| `member@aratuwa.edu` | `981a7340-2a25-4aac-8b49-fddf45ff4894` | University of Aratuwa |
| `platform@corta.dev` | `d47b9e21-5a13-4c88-b0e7-8391f6a2d504` | None; platform operations do not grant tenant content access |

The seeded organization public IDs are:

| Organization | Public ID |
| --- | --- |
| University of Aratuwa | `30ee7153-9b48-4560-8cbf-972587a60fda` |
| MedSync | `f1810095-f8a0-4e27-83df-d88b3256604d` |
| Pied Piper | `afb118ba-2ade-4422-9f20-04754fd1d4a7` |

Legacy seed memberships intentionally have no guessed owner. Before using an
administrative organization route, assign the intended owner and verify the
cutover precondition:

```bash
make assign-org-owner \
  ORG_ID=30ee7153-9b48-4560-8cbf-972587a60fda \
  USER_ID=0d5a4f4e-8d3b-4f17-9a79-4c38e29a6d11
make verify-org-owners
```

The verify command prints only public IDs for active ownerless organizations
and exits nonzero until every one has an owner.

The API uses an opaque cookie session. The login response also returns the CSRF
token required with an approved exact `Origin` on unsafe requests. This example
uses a temporary cookie jar; do not commit it:

```bash
COOKIE_JAR="$(mktemp)"
LOGIN_RESPONSE="$(curl -sS -c "${COOKIE_JAR}" \
  -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@aratuwa.edu","password":"synodus-demo-password"}')"
CSRF_TOKEN="$(printf '%s' "${LOGIN_RESPONSE}" | jq -r '.csrf_token')"

curl -sS -b "${COOKIE_JAR}" http://localhost:8080/api/v1/auth/session
curl -sS -b "${COOKIE_JAR}" http://localhost:8080/api/v1/orgs
```

When using the shared Envoy entrypoint, open the app at
`http://localhost:10000` and include `Origin: http://localhost:10000` on direct
unsafe API calls. `HTTP_ALLOWED_ORIGINS` must include that exact origin.

The seeds do not create teams. An organization owner or administrator can create
one after the tenant provisioner reports the organization current:

```bash
curl -sS -b "${COOKIE_JAR}" \
  -X POST http://localhost:8080/api/v1/orgs/30ee7153-9b48-4560-8cbf-972587a60fda/teams \
  -H 'Origin: http://localhost:3000' \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Lab Alpha"}'
```

## Tenant provisioning operations

Creating or restoring an organization records durable provisioning intent and
returns immediately. The dedicated provisioner creates/adopts its canonical
schema and applies the embedded tenant migration set. Tenant routes remain
unavailable until the organization is active at the exact embedded version and
checksum.

```bash
go run ./cmd/provisioner run
go run ./cmd/provisioner status --all
go run ./cmd/provisioner status --organization ORGANIZATION_UUID
go run ./cmd/provisioner reconcile --organization ORGANIZATION_UUID
go run ./cmd/provisioner reconcile --all --concurrency 4
go run ./cmd/provisioner retry --organization ORGANIZATION_UUID
go run ./cmd/provisioner retry --all
make migrate-status
```

Commands accept public organization UUIDs only and write one bounded JSON
object per line. `status` and `reconcile` exit nonzero if any selected tenant is
not current. Transient reconciliation failures retry automatically up to five
attempts with persisted exponential backoff; permanent catalog/checksum
divergence fails immediately and requires operator repair followed by `retry`.

Apply public migrations before deploying a new API/provisioner. Stop the old
provisioner before starting a binary with a different embedded migration set.
Before deploying application code that depends on the new tenant migration set,
require `provisioner status --all` to report `"current":true` for every
non-deleting organization. Back up the public registry before first adopting an
existing legacy tenant fleet.

## File storage

MinIO remains a configured and readiness-checked dependency for the M05 storage
work. The authenticated v1 API deliberately exposes no file HTTP routes yet;
metadata-backed authorization and bounded transfer semantics must land before
uploads or downloads become public.

## Document collaboration

Team Members request a short-lived Document Room ticket from
`POST /api/v1/orgs/{org_id}/teams/{team_id}/documents/{document_id}/socket-ticket`.
The operation requires the browser session's CSRF token and verifies that the
Document belongs to the requested team before signing the user, organization,
team, and Document scope with `JWT_SECRET`. The collaboration process validates
that ticket locally before loading a room. It then uses the private
`GET|PUT /internal/v1/orgs/{org_id}/teams/{team_id}/documents/{document_id}/state`
operations with `COLLABORATION_SERVICE_SECRET` and the ticket's Editor identity.
The private calls recheck team membership and Document permission; the
collaboration service never receives tenant database credentials.

The browser connects through Envoy at
`ws://localhost:10000/ws/docs?org_id={org_id}&team_id={team_id}`, using
`{org_id}:{team_id}:{document_id}` as the Hocuspocus Document name and the
issued token as its connection token. The collaboration process reports ready
only when core-api and Redis room lifecycle checks succeed:

```bash
curl -sS http://localhost:8082/health
# {"ok":true,"service":"collaboration-server"}
```

The reviewed public route inventory is below. `none` means that the request is
not rate-limited by an application policy; infrastructure-wide controls may
still apply. Routes with a `none` body reject a supplied request body.

| Operation | Success | Error statuses | Permission | CSRF | Body limit | Rate limit |
| --- | --- | --- | --- | --- | --- | --- |
| `GET .../documents` | 200 | 401, 403, 404, 500, 503 | `document.read` | no | none | none |
| `POST .../documents` | 201 | 400, 401, 403, 404, 500, 503 | `document.create` | yes | 64 KiB JSON | none |
| `GET .../documents/{document_id}` | 200 | 401, 403, 404, 500, 503 | `document.read` | no | none | none |
| `PATCH .../documents/{document_id}` | 200 | 400, 401, 403, 404, 500, 503 | `document.update` | yes | 64 KiB JSON | none |
| `DELETE .../documents/{document_id}` | 204 | 401, 403, 404, 500, 503 | `document.delete` | yes | none | none |
| `POST .../documents/{document_id}/socket-ticket` | 200 | 401, 403, 404, 500, 503 | `realtime.connect` | yes | none | none |
| `POST .../chat/socket-ticket` | 200 | 401, 403, 404, 500, 503 | `realtime.connect` | yes | none | none |

Every public route uses the browser session cookie. A 403 is returned when the
authenticated user lacks the listed team permission; unknown, cross-team, and
cross-organization resources are concealed as 404 where appropriate. Error
bodies use `application/problem+json`. `api/openapi.yaml` is authoritative and
the executable inventory in `internal/apicontract/inventory.go` is checked
against it.

The private state operations both require service bearer authentication and an
`X-Synodus-Editor-ID`; they have no CSRF or rate-limit policy. `GET .../state`
accepts no body and returns 200, while `PUT .../state` accepts up to 16 MiB of
JSON and returns 200. Both declare 400, 401, 403, 404, 500, and 503 errors.

### Backup and restore expectations

Document title, HTML projection, and canonical Yjs state live in each tenant's
Postgres `documents` table. Ordinary tenant-database backups therefore include
Document content; restore it with the same database backup and restore process
used for the rest of that tenant schema. Redis holds no authoritative Document
content and is not a Document backup source.

The application keeps only the latest canonical state. It has no custom
Document version history, point-in-time Document restore, or application-level
recovery protocol. Database-level point-in-time recovery, if configured by the
operator, remains an infrastructure capability rather than a product feature.

### First-release limits

- Document Rooms run as a single collaboration-service replica. Distributed
  room ownership and Redis-backed horizontal scaling are deferred.
- Hocuspocus/Yjs protocol behavior is used as shipped; there is no custom
  per-change durable acknowledgement or immediate membership-revocation push.
  Authorization is rechecked when a ticket is issued and when state is loaded
  or stored.
- Every Team Member can read, create, and edit Documents; Team Admins and
  Research Leads can also delete. Granular per-Document roles are deferred.
- The editor supports the current title and rich-text paragraph/formatting
  schema. Richer editor nodes and an application-level version history are out
  of scope.

## Realtime chat

Team chat history, send, delete, and socket-ticket routes live in core-api.
Each chat write commits to the tenant database, then publishes a small event to
Redis on `REDIS_CHAT_CHANNEL` (`corta:chat:events` by default). The separate
socket-server subscribes to that channel and fans events out to connected
browser WebSockets for the matching team room.

Set the same `JWT_SECRET` for core-api and socket-server. Core-api uses it to
issue short-lived socket tickets from
`POST /api/v1/orgs/{org_id}/teams/{team_id}/chat/socket-ticket`; socket-server
validates those tickets locally when the browser connects to
`ws://localhost:8081/ws?token=<socket_ticket>&team_id=<team_uuid>`.
Through Envoy, use
`ws://localhost:10000/ws?token=<socket_ticket>&team_id=<team_uuid>` instead.

Redis is still used for shared login and administrative rate limits. WebSockets
alone are not enough once there can be more than one socket-server replica,
because a message sent through core-api must reach clients connected to any
replica.

The project uses `sqlc` for code generation using migration files.
