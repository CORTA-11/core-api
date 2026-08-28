SHELL := /usr/bin/env bash

CACHE_DIR := $(CURDIR)/.cache
TOOLS_DIR := $(CACHE_DIR)/tools
GOCACHE := $(CACHE_DIR)/go-build
GOLANGCI_LINT_CACHE := $(CACHE_DIR)/golangci-lint
GOFLAGS ?= -buildvcs=false

export GOCACHE GOLANGCI_LINT_CACHE GOFLAGS

include tools.mk

.PHONY: check build fmt fmt-check mod-check static vet lint diagnostics sec secrets migrations-check queries-check contract-check \
	test test-unit test-race test-integration test-isolation test-contract generate generate-check \
	bootstrap-db migrate-up-all migrate-down-all migrate-up migrate-down migrate-status seed run provisioner bootstrap assign-org-owner verify-org-owners tools clean-tools

check: fmt-check mod-check build generate-check migrations-check queries-check contract-check static sec

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

contract-check:
	go test ./internal/apicontract ./internal/httpx ./internal/pagination

test-contract: contract-check
	./scripts/test-integration.sh contract

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.cache/*')

fmt-check:
	@files="$$(gofmt -l $$(find . -name '*.go' -not -path './.cache/*'))"; \
	if [[ -n "$$files" ]]; then echo "Go files need formatting:"; echo "$$files"; exit 1; fi

mod-check:
	./scripts/mod-check.sh

# Analyze every Go package and test with compiler/vet, Staticcheck-backed
# linting, and the same diagnostics developers see through gopls editors.
static: vet lint diagnostics

vet:
	go vet ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

diagnostics: $(GOPLS)
	GOPLS="$(GOPLS)" ./scripts/check-go-diagnostics.sh

sec: $(GOVULNCHECK) $(GOSEC)
	$(GOVULNCHECK) ./...
	$(GOSEC) -quiet -exclude-generated -exclude-dir=.cache ./...

secrets: $(GITLEAKS)
	$(GITLEAKS) git --redact --no-banner

migrations-check:
	./scripts/check-migrations.sh

queries-check:
	go run ./cmd/querycheck

generate: $(SQLC)
	$(SQLC) generate

generate-check: $(SQLC)
	SQLC="$(SQLC)" ./scripts/generate-check.sh

tools: $(SQLC) $(GOLANGCI_LINT) $(GOSEC) $(GOVULNCHECK) $(GITLEAKS) $(GOPLS)

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

$(GOPLS):
	@mkdir -p "$(@D)"
	GOBIN="$(@D)" go install golang.org/x/tools/gopls@$(GOPLS_VERSION)

clean-tools:
	rm -rf "$(TOOLS_DIR)"

# Runtime targets alone load local environment values. Explicit URLs win.
RUNTIME_GOALS := run provisioner seed bootstrap bootstrap-db migrate-up-all migrate-down-all migrate-up migrate-down migrate-status assign-org-owner verify-org-owners
ifneq ($(filter $(RUNTIME_GOALS),$(MAKECMDGOALS)),)
-include .env
DATABASE_URL ?= postgres://synodus_runtime:$(DB_RUNTIME_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable
MIGRATION_DATABASE_URL ?= postgres://synodus_migrator:$(DB_MIGRATOR_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable
PROVISIONING_DATABASE_URL ?= postgres://synodus_provisioner:$(DB_PROVISIONER_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable
BOOTSTRAP_DATABASE_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable
REDIS_URL ?= redis://$(if $(REDIS_HOST),$(REDIS_HOST),localhost):$(if $(REDIS_PORT),$(REDIS_PORT),6379)/0
unexport DATABASE_URL MIGRATION_DATABASE_URL PROVISIONING_DATABASE_URL BOOTSTRAP_DATABASE_URL
unexport DB_USER DB_PASSWORD DB_RUNTIME_PASSWORD DB_MIGRATOR_PASSWORD DB_PROVISIONER_PASSWORD
export APP_ENV HTTP_ADDR HTTP_ALLOWED_ORIGINS HTTP_TRUSTED_PROXY_CIDRS HTTP_READ_HEADER_TIMEOUT HTTP_READ_TIMEOUT HTTP_WRITE_TIMEOUT HTTP_IDLE_TIMEOUT
export SHUTDOWN_TIMEOUT DEPENDENCY_TIMEOUT PPROF_ENABLED PPROF_ADDR
export PROVISIONER_POLL_INTERVAL PROVISIONER_RETRY_INITIAL PROVISIONER_RETRY_MAXIMUM
export PROVISIONER_MAX_ATTEMPTS PROVISIONER_CONCURRENCY PROVISIONER_OPERATION_TIMEOUT
export PROVISIONER_SHUTDOWN_TIMEOUT
export REDIS_URL RATE_LIMIT_SECRET RATE_LIMIT_TIMEOUT RATE_LIMIT_LOGIN_IP_LIMIT RATE_LIMIT_LOGIN_IP_WINDOW RATE_LIMIT_LOGIN_IP_BURST
export RATE_LIMIT_REGISTRATION_IP_LIMIT RATE_LIMIT_REGISTRATION_IP_WINDOW RATE_LIMIT_REGISTRATION_IP_BURST
export RATE_LIMIT_ACCOUNT_FAILURE_LIMIT RATE_LIMIT_ACCOUNT_FAILURE_WINDOW RATE_LIMIT_ACCOUNT_FAILURE_BURST
export RATE_LIMIT_ADMIN_LIMIT RATE_LIMIT_ADMIN_WINDOW RATE_LIMIT_ADMIN_BURST CSRF_SECRET CURSOR_KEY_ID CURSOR_SECRET
export CURSOR_PREVIOUS_KEY_ID CURSOR_PREVIOUS_SECRET
export MINIO_ENDPOINT MINIO_ACCESS_KEY MINIO_SECRET_KEY MINIO_BUCKET_NAME MINIO_USE_SSL
endif

ifneq ($(filter run bootstrap assign-org-owner verify-org-owners,$(MAKECMDGOALS)),)
export DATABASE_URL
endif

ifneq ($(filter provisioner,$(MAKECMDGOALS)),)
export PROVISIONING_DATABASE_URL
endif

ifneq ($(filter seed migrate-up-all migrate-down-all migrate-up migrate-down migrate-status,$(MAKECMDGOALS)),)
export MIGRATION_DATABASE_URL
endif

ifneq ($(filter bootstrap-db,$(MAKECMDGOALS)),)
export BOOTSTRAP_DATABASE_URL DB_RUNTIME_PASSWORD DB_MIGRATOR_PASSWORD DB_PROVISIONER_PASSWORD
endif

PUBLIC_MIGRATION_PATH := ./cmd/migrate

bootstrap-db: export MIGRATION_DATABASE_URL := $(BOOTSTRAP_DATABASE_URL)
bootstrap-db:
	go run "$(PUBLIC_MIGRATION_PATH)" up-all
	go run ./cmd/dbroles

migrate-up-all:
	go run "$(PUBLIC_MIGRATION_PATH)" up-all

migrate-down-all:
	go run "$(PUBLIC_MIGRATION_PATH)" down-all

migrate-up:
	go run "$(PUBLIC_MIGRATION_PATH)" up

migrate-down:
	go run "$(PUBLIC_MIGRATION_PATH)" down

migrate-status:
	go run "$(PUBLIC_MIGRATION_PATH)" status

seed:
	go run ./cmd/seed

run:
	go run ./cmd/api

provisioner:
	go run ./cmd/provisioner run

bootstrap:
	go run ./cmd/bootstrap

verify-org-owners:
	go run ./cmd/admin org owner verify

assign-org-owner:
	@test -n "$(ORG_ID)"
	@test -n "$(USER_ID)"
	go run ./cmd/admin org owner assign --org "$(ORG_ID)" --user "$(USER_ID)"
