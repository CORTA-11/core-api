# core-api

## Development

1. Create a .env file with database credentials
2. Start postgres: `docker compose up`
3. Run schema migrations: `make migrate-up-all`
4. If needed, populate db with seed data: `make seed`
5. Start the server: `make run`

The project uses `sqlc` for code generation using migration files.