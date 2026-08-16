# core-api

## Development

1. Copy env template:
   ```bash
   cp .env.example .env
   ```
2. Start Postgres, Redis, and MinIO:
   ```bash
   docker compose up -d
   ```
3. Run schema migrations: `make migrate-up-all`
4. If needed, seed data: `make seed`
5. Start socket-server (subscribes to Redis):
   ```bash
   cd ../socket-server && cp -n .env.example .env && make run
   ```
6. Start this API: `make run`

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

Demo seed users (password `password123`): `admin@aratuwa.edu`, `leader@aratuwa.edu`,
`member@aratuwa.edu` — team **Lab Alpha**.

The project uses `sqlc` for code generation using migration files.
