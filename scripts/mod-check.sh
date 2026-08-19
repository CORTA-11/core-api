#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
temp_dir="$(mktemp -d)"
trap 'rm -rf "${temp_dir}"' EXIT
cp "${repo_root}/go.mod" "${repo_root}/go.sum" "${temp_dir}/"
cp -R "${repo_root}/cmd" "${repo_root}/internal" "${temp_dir}/"
(cd "${temp_dir}" && go mod tidy)
diff -u "${repo_root}/go.mod" "${temp_dir}/go.mod"
diff -u "${repo_root}/go.sum" "${temp_dir}/go.sum"
