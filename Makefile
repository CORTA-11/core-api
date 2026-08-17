include .env

# ===========================
# TODO: REMOVE BEFORE PROD
# ===========================
DATABASE_URL := postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable

INTERNAL_API_KEY ?= dev-internal-key-change-me
JWT_SECRET ?= your-super-secret-key-change-in-production
REDIS_URL ?= redis://localhost:6379/0
REDIS_CHAT_CHANNEL ?= corta:chat:events

test:
	go test -v ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

sec:
	govulncheck ./...
	gosec -exclude-generated ./...

PUBLIC_MIGRATION_PATH=./cmd/migrate
# Migrate schema to the most up-to-date version
migrate-up-all:
	DATABASE_URL="$(DATABASE_URL)" go run "$(PUBLIC_MIGRATION_PATH)" up-all

# Remove all schema migrations
migrate-down-all:
	DATABASE_URL="$(DATABASE_URL)" go run "$(PUBLIC_MIGRATION_PATH)" down-all

# Migrate schema up one version
migrate-up:
	DATABASE_URL="$(DATABASE_URL)" go run "$(PUBLIC_MIGRATION_PATH)" up

# Remove the last migration
migrate-down:
	DATABASE_URL="$(DATABASE_URL)" go run "$(PUBLIC_MIGRATION_PATH)" down

# Fill tables with seed data
seed:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/seed

# Run the server
run:
	DATABASE_URL="$(DATABASE_URL)" \
	INTERNAL_API_KEY="$(INTERNAL_API_KEY)" \
	JWT_SECRET="$(JWT_SECRET)" \
	REDIS_URL="$(REDIS_URL)" \
	REDIS_CHAT_CHANNEL="$(REDIS_CHAT_CHANNEL)" \
	MINIO_ENDPOINT="$(MINIO_ENDPOINT)" \
	MINIO_ACCESS_KEY="$(MINIO_ACCESS_KEY)" \
	MINIO_SECRET_KEY="$(MINIO_SECRET_KEY)" \
	MINIO_BUCKET_NAME="$(MINIO_BUCKET)" \
	MINIO_USE_SSL="$(MINIO_USE_SSL)" \
	REDIS_HOST="$(REDIS_HOST)" \
	REDIS_PORT="$(REDIS_PORT)" \
	go run ./cmd/api
