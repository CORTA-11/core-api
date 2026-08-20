#!/usr/bin/env bash
set -euo pipefail

suite="${1:-integration}"
case "${suite}" in
  integration|isolation) ;;
  *) echo "usage: $0 [integration|isolation]" >&2; exit 2 ;;
esac

compose_file="docker-compose.test.yaml"
project="core-api-${suite}-$(date +%s)-$$"
compose=(docker compose --project-name "${project}" --file "${compose_file}")

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

"${compose[@]}" up --detach --wait --wait-timeout 120
postgres_port="$("${compose[@]}" port postgres 5432 | awk -F: 'END {print $NF}')"
redis_port="$("${compose[@]}" port redis 6379 | awk -F: 'END {print $NF}')"
minio_port="$("${compose[@]}" port minio 9000 | awk -F: 'END {print $NF}')"

export TEST_DATABASE_URL="postgres://integration:integration-password@127.0.0.1:${postgres_port}/core_api_test?sslmode=disable"
export TEST_REDIS_URL="redis://127.0.0.1:${redis_port}/0"
export TEST_MINIO_ENDPOINT="127.0.0.1:${minio_port}"
export TEST_MINIO_ACCESS_KEY="integration"
export TEST_MINIO_SECRET_KEY="integration-password"

go test -count=1 -tags="${suite}" ./internal/integration/...
