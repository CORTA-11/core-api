#!/usr/bin/env bash
set -euo pipefail

: "${SQLC:?SQLC must point to the pinned sqlc binary}"
repo_root="$(git rev-parse --show-toplevel)"
temp_dir="$(mktemp -d)"
trap 'rm -rf "${temp_dir}"' EXIT
mkdir -p "${temp_dir}/db" "${temp_dir}/internal"
cp -R "${repo_root}/db/migrations" "${repo_root}/db/queries" "${temp_dir}/db/"
cp -R "${repo_root}/internal/repository" "${temp_dir}/internal/"
cp "${repo_root}/sqlc.yml" "${temp_dir}/"
(cd "${temp_dir}" && "${SQLC}" generate)
diff -ru --exclude='*_test.go' "${repo_root}/internal/repository" "${temp_dir}/internal/repository"
