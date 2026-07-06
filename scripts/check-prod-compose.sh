#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="${CHATDOCK_CONTAINER_NAME:-chatdock}"
EXPECTED_DIR="${CHATDOCK_PROD_DIR:-/Volumes/KIOXIA/Docker/chatdock}"
EXPECTED_COMPOSE="$EXPECTED_DIR/compose.yaml"
EXPECTED_DATA="$EXPECTED_DIR/data"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found" >&2
  exit 1
fi

if ! docker inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
  echo "container not found: $CONTAINER_NAME" >&2
  exit 1
fi

working_dir="$(docker inspect "$CONTAINER_NAME" --format '{{index .Config.Labels "com.docker.compose.project.working_dir"}}')"
config_files="$(docker inspect "$CONTAINER_NAME" --format '{{index .Config.Labels "com.docker.compose.project.config_files"}}')"
data_mount="$(docker inspect "$CONTAINER_NAME" --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Source}}{{end}}{{end}}')"

ok=1
if [[ "$working_dir" != "$EXPECTED_DIR" ]]; then
  echo "unexpected compose working_dir: $working_dir, expected: $EXPECTED_DIR" >&2
  ok=0
fi
if [[ "$config_files" != "$EXPECTED_COMPOSE" ]]; then
  echo "unexpected compose config_files: $config_files, expected: $EXPECTED_COMPOSE" >&2
  ok=0
fi
if [[ "$data_mount" != "$EXPECTED_DATA" ]]; then
  echo "unexpected /data mount: $data_mount, expected: $EXPECTED_DATA" >&2
  ok=0
fi

if [[ "$ok" != 1 ]]; then
  exit 1
fi

echo "ChatDock production compose verified: $EXPECTED_COMPOSE, /data -> $EXPECTED_DATA"
