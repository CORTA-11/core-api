include .env

# ===========================
# TODO: REMOVE BEFORE PROD
# ===========================
DATABASE_URL := postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

sec:
	govulncheck ./...
	gosec ./...

# Migrate schema to the most up-to-date version
migrate-up-all:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate up-all

# Remove all schema migrations
migrate-down-all:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate down-all

# Migrate schema up one version
migrate-up:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate up

# Remove the last migration
migrate-down:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate down

# Fill tables with seed data
seed:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/seed

# Run the server
run:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/api/main.go