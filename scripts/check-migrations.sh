#!/usr/bin/env bash
set -euo pipefail

status=0
for directory in db/migrations/public db/migrations/tenant; do
  declare -A up=() down=()
  while IFS= read -r -d '' file; do
    name="$(basename "${file}")"
    if [[ "${name}" =~ ^([0-9]{6})_[a-z0-9_]+\.(up|down)\.sql$ ]]; then
      version="${BASH_REMATCH[1]}"
      direction="${BASH_REMATCH[2]}"
      if [[ "${direction}" == up ]]; then up["${version}"]="${name}"; else down["${version}"]="${name}"; fi
    else
      echo "invalid migration filename: ${file}" >&2
      status=1
    fi
  done < <(find "${directory}" -maxdepth 1 -type f -name '*.sql' -print0 | sort -z)
  for version in "${!up[@]}"; do
    [[ -n "${down[${version}]:-}" ]] || { echo "missing down migration for ${directory}/${up[${version}]}" >&2; status=1; }
  done
  for version in "${!down[@]}"; do
    [[ -n "${up[${version}]:-}" ]] || { echo "missing up migration for ${directory}/${down[${version}]}" >&2; status=1; }
  done
  unset up down
done
exit "${status}"
