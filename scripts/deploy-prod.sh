#!/usr/bin/env bash
set -euo pipefail

PROD_DIR="${CHATDOCK_PROD_DIR:-/Volumes/KIOXIA/Docker/chatdock}"
PROD_COMPOSE="$PROD_DIR/compose.yaml"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"

if [[ ! -f "$PROD_COMPOSE" ]]; then
  echo "production compose not found: $PROD_COMPOSE" >&2
  exit 1
fi

if [[ "$PWD" == "$REPO_DIR" ]]; then
  echo "refusing source-directory production deploy from $REPO_DIR" >&2
  echo "use: make deploy-prod" >&2
  exit 1
fi

docker compose -f "$PROD_COMPOSE" --project-directory "$PROD_DIR" up -d --build chatdock
"$REPO_DIR/scripts/check-prod-compose.sh"
