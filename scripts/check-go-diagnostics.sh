#!/usr/bin/env bash
set -euo pipefail

: "${GOPLS:?GOPLS must point to the pinned gopls binary}"

mapfile -d '' -t go_files < <(
  git ls-files --cached --others --exclude-standard -z -- \
    '*.go' ':(exclude).cache/**' \
    | sort -zu
)

if [[ "${#go_files[@]}" -eq 0 ]]; then
  echo "no Go files found" >&2
  exit 1
fi

"${GOPLS}" check "${go_files[@]}"
