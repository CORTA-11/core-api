SHELL := /usr/bin/env bash

CACHE_DIR := $(CURDIR)/.cache
TOOLS_DIR := $(CACHE_DIR)/tools
GOCACHE := $(CACHE_DIR)/go-build
GOLANGCI_LINT_CACHE := $(CACHE_DIR)/golangci-lint
GOFLAGS ?= -buildvcs=false

export GOCACHE GOLANGCI_LINT_CACHE GOFLAGS

include tools.mk

.PHONY: check build fmt fmt-check mod-check lint sec secrets migrations-check \
	test test-unit test-race test-integration test-isolation generate generate-check \
	migrate-up-all migrate-down-all migrate-up migrate-down seed run bootstrap tools clean-tools

check: fmt-check mod-check build generate-check migrations-check lint sec

build:
	go build ./...

test: test-unit

test-unit:
	go test -v ./...

test-race:
	go test -race ./...

test-integration:
	./scripts/test-integration.sh integration

test-isolation:
	./scripts/test-integration.sh isolation

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.cache/*')

fmt-check:
	@files="$$(gofmt -l $$(find . -name '*.go' -not -path './.cache/*'))"; \
	if [[ -n "$$files" ]]; then echo "Go files need formatting:"; echo "$$files"; exit 1; fi

mod-check:
	./scripts/mod-check.sh

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

sec: $(GOVULNCHECK) $(GOSEC)
	$(GOVULNCHECK) ./...
	$(GOSEC) -quiet -exclude-generated -exclude-dir=.cache ./...

secrets: $(GITLEAKS)
	$(GITLEAKS) git --redact --no-banner

migrations-check:
	./scripts/check-migrations.sh

generate: $(SQLC)
	$(SQLC) generate

generate-check: $(SQLC)
	SQLC="$(SQLC)" ./scripts/generate-check.sh

tools: $(SQLC) $(GOLANGCI_LINT) $(GOSEC) $(GOVULNCHECK) $(GITLEAKS)

$(SQLC):
	@mkdir -p "$(@D)"
	GOBIN="$(@D)" go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

$(GOLANGCI_LINT):
	@mkdir -p "$(@D)"
	GOBIN="$(@D)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOSEC):
	@mkdir -p "$(@D)"
	GOBIN="$(@D)" go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)

$(GOVULNCHECK):
	@mkdir -p "$(@D)"
	GOBIN="$(@D)" go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(GITLEAKS):
	@mkdir -p "$(@D)"
	GOBIN="$(@D)" go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

clean-tools:
	rm -rf "$(TOOLS_DIR)"

# Runtime targets alone load local environment values. Explicit URLs win.
RUNTIME_GOALS := run seed bootstrap migrate-up-all migrate-down-all migrate-up migrate-down
ifneq ($(filter $(RUNTIME_GOALS),$(MAKECMDGOALS)),)
-include .env
endif
DATABASE_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable
REDIS_URL ?= redis://$(if $(REDIS_HOST),$(REDIS_HOST),localhost):$(if $(REDIS_PORT),$(REDIS_PORT),6379)/0

PUBLIC_MIGRATION_PATH := ./cmd/migrate

migrate-up-all:
	DATABASE_URL="$(DATABASE_URL)" go run "$(PUBLIC_MIGRATION_PATH)" up-all

migrate-down-all:
	DATABASE_URL="$(DATABASE_URL)" go run "$(PUBLIC_MIGRATION_PATH)" down-all

migrate-up:
	DATABASE_URL="$(DATABASE_URL)" go run "$(PUBLIC_MIGRATION_PATH)" up

migrate-down:
	DATABASE_URL="$(DATABASE_URL)" go run "$(PUBLIC_MIGRATION_PATH)" down

seed:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/seed

run:
	DATABASE_URL="$(DATABASE_URL)" REDIS_URL="$(REDIS_URL)" go run ./cmd/api

bootstrap:
	DATABASE_URL="$(DATABASE_URL)" REDIS_URL="$(REDIS_URL)" go run ./cmd/bootstrap
