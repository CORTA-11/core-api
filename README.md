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

All seeded users use the development-only password `synodus-demo-password`.
Each account stores a distinct target-parameter hash.

| User | Public user ID | Organization memberships |
| --- | --- | --- |
| `admin@aratuwa.edu` | `0d5a4f4e-8d3b-4f17-9a79-4c38e29a6d11` | University of Aratuwa, MedSync, Pied Piper |
| `leader@aratuwa.edu` | `48b38b47-36a8-4758-9858-c28c222d2c2e` | University of Aratuwa, MedSync |
| `member@aratuwa.edu` | `981a7340-2a25-4aac-8b49-fddf45ff4894` | University of Aratuwa |

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

## Realtime (SaaS-ready fan-out)

Realtime HTTP and WebSocket product routes are not part of the M03 API surface.
Redis is currently used by core-api for shared login and administrative rate
limits. The later collaboration milestone will add authorization-aware event
publication and socket integration.

The project uses `sqlc` for code generation using migration files.
