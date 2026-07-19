#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="${CHATDOCK_CONTAINER_NAME:-chatdock}"
HEALTH_URL="${CHATDOCK_HEALTH_URL:-http://127.0.0.1:8720/api/health}"
MAX_ATTEMPTS="${CHATDOCK_HEALTH_ATTEMPTS:-30}"
RETRY_SECONDS="${CHATDOCK_HEALTH_RETRY_SECONDS:-1}"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found" >&2
  exit 1
fi
if ! docker inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
  echo "container not found: $CONTAINER_NAME" >&2
  exit 1
fi

health_token="${CHATDOCK_HEALTH_TOKEN:-}"
if [[ -z "$health_token" ]]; then
  while IFS= read -r entry; do
    case "$entry" in
      CHATDOCK_AUTH_TOKEN=*) health_token="${entry#*=}" ;;
    esac
  done < <(docker inspect "$CONTAINER_NAME" --format '{{range .Config.Env}}{{println .}}{{end}}')
fi

curl_config="$(mktemp)"
trap 'rm -f "$curl_config"' EXIT
chmod 0600 "$curl_config"
if [[ -n "$health_token" ]]; then
  printf 'header = "Authorization: Bearer %s"\n' "$health_token" >"$curl_config"
fi

last_error=""
for ((attempt = 1; attempt <= MAX_ATTEMPTS; attempt++)); do
  response=""
  if response="$(curl --config "$curl_config" --fail --silent --show-error --max-time 3 "$HEALTH_URL" 2>&1)"; then
    if printf '%s' "$response" | python3 -c 'import json, sys; payload = json.load(sys.stdin); raise SystemExit(0 if payload.get("ok") is True and payload.get("name") == "ChatDock" else 1)' 2>/dev/null; then
      echo "ChatDock production health verified: $HEALTH_URL"
      exit 0
    fi
    last_error="unexpected health response"
  else
    last_error="$response"
  fi
  if ((attempt < MAX_ATTEMPTS)); then
    sleep "$RETRY_SECONDS"
  fi
done

echo "ChatDock production health check failed after $MAX_ATTEMPTS attempts: $last_error" >&2
exit 1
