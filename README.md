# core-api

## Development

1. Copy env template:
   ```bash
   cp .env.example .env
   ```
2. Start Postgres + Redis:
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
