#!/usr/bin/env bash
# Runs the Engagement Entity (:8290) and the Engagement App Backend (:8190)
# together for local development. Each reads its settings from its own
# git-ignored .env (copy .env.local to .env first). Ctrl-C stops both.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
entity="$here/../../entity/engagement-entity"
backend="$here/backend"

for dir in "$entity" "$backend"; do
  if [[ ! -f "$dir/.env" ]]; then
    echo "Missing $dir/.env: copy $dir/.env.local to .env and fill it in." >&2
    exit 1
  fi
done

trap 'kill 0' EXIT INT TERM
(cd "$entity" && go run ./cmd/api) &
(cd "$backend" && go run ./cmd/server) &
wait
