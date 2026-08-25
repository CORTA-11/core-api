# core-api

## Development

1. Copy the environment template and choose distinct development passwords:

   ```bash
   cp .env.example .env
   ```

   If `.env` already exists, keep its working `DB_USER`/`DB_PASSWORD` values
   and add `DB_RUNTIME_PASSWORD`, `DB_MIGRATOR_PASSWORD`, and
   `DB_PROVISIONER_PASSWORD`. These three values become the PostgreSQL passwords
   for their matching roles.

2. Start Postgres, Redis, and MinIO:

   ```bash
   docker compose up -d
   ```

3. Bootstrap the database roles and apply public migrations:

   ```bash
   make bootstrap-db
   ```

   This one-time/recovery command uses `DB_USER`/`DB_PASSWORD` as administrator
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

   Verify startup with `curl -i http://localhost:8080/health/ready`; a ready
   development stack returns HTTP 204.

7. Start socket-server if realtime behavior is needed:

   ```bash
   cd ../socket-server && cp -n .env.example .env && make run
   ```

For later public migrations use `make migrate-up-all`; do not rerun them with
runtime credentials. `make bootstrap-db` is also the recovery command when an
existing development `.env` receives new role passwords. If a password contains
URL-reserved characters, set URL-encoded `BOOTSTRAP_DATABASE_URL`,
`DATABASE_URL`, `MIGRATION_DATABASE_URL`, and `PROVISIONING_DATABASE_URL`
explicitly instead of relying on the component-derived development URLs.

### Development seed data

All seeded users use the password `password123`.

| User | Public user ID | Organization memberships |
| --- | --- | --- |
| `admin@aratuwa.edu` | `0d5a4f4e-8d3b-4f17-9a79-4c38e29a6d11` | University of Aratuwa, MedSync, Pied Piper |
| `leader@aratuwa.edu` | `48b38b47-36a8-4758-9858-c28c222d2c2e` | University of Aratuwa, MedSync |
| `member@aratuwa.edu` | `981a7340-2a25-4aac-8b49-fddf45ff4894` | University of Aratuwa |

The seeded organization selectors are:

| Organization | `X-Org-ID` |
| --- | --- |
| University of Aratuwa | `30ee7153-9b48-4560-8cbf-972587a60fda` |
| MedSync | `f1810095-f8a0-4e27-83df-d88b3256604d` |
| Pied Piper | `afb118ba-2ade-4422-9f20-04754fd1d4a7` |

Log in to obtain a bearer token:

```bash
curl -X POST http://localhost:8080/users/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@aratuwa.edu","password":"password123"}'
```

The seeds do not create teams. After assigning the response token to the
`API_TOKEN` shell variable, create one as the selected organization's
authenticated member:

```bash
curl -X POST http://localhost:8080/teams \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -H 'X-Org-ID: 30ee7153-9b48-4560-8cbf-972587a60fda' \
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

Files are stored in the shared MinIO bucket configured by `MINIO_BUCKET_NAME`.
Organization and team isolation is provided by hierarchical object keys:

```text
orgs/{organization-uuid}/teams/{team-id}/files/{filename}
```

The file endpoints require an existing organization UUID in the `X-Org-ID`
header and an existing team slug in the URL.

Upload a file:

```bash
curl -X POST \
  -H "X-Org-ID: YOUR_ORG_UUID" \
  -F "file=@./report.pdf" \
  http://localhost:8080/YOUR_TEAM_SLUG/files/upload
```

Download the file:

```bash
curl -f \
  -H "X-Org-ID: YOUR_ORG_UUID" \
  -o downloaded-report.pdf \
  http://localhost:8080/YOUR_TEAM_SLUG/files/download/report.pdf
```

The MinIO console is available at `http://localhost:9001` when the Docker
Compose services are running. Uploading the same filename again within the
same organization and team replaces the existing object.

## Realtime (SaaS-ready fan-out)

After a chat message is saved to Postgres, core-api publishes to Redis:

```text
POST /teams/.../messages  →  Postgres  →  Redis PUBLISH corta:chat:events
                                              ↓
                                    socket-server replica(s) → WebSocket clients
```

Configure via `REDIS_URL` and `REDIS_CHAT_CHANNEL` (see `.env.example`).
`INTERNAL_API_KEY` / `JWT_SECRET` must match socket-server.

The project uses `sqlc` for code generation using migration files.
