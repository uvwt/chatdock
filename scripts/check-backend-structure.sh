#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "backend structure check failed: $*" >&2
  exit 1
}

[[ ! -e internal/chatdock ]] || fail "internal/chatdock must not exist"

for package_dir in app httpapi model store; do
  [[ -d "internal/${package_dir}" ]] || fail "missing internal/${package_dir}"
done

if find internal -maxdepth 1 -type f -name '*.go' -print -quit | grep -q .; then
  fail "Go files must belong to a named package directory under internal"
fi

if grep -RIl --include='*.go' --include='*.md' --include='*.mjs' --include='*.sh' \
  --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist \
  'chatdock/internal/chatdock' . | grep -v '^./scripts/check-backend-structure.sh$' | grep -q .; then
  fail "stale chatdock/internal/chatdock import or documentation path"
fi

grep -q '"chatdock/internal/app"' cmd/chatdock/main.go || fail "cmd/chatdock must enter through internal/app"
grep -q '"chatdock/internal/httpapi"' internal/app/run.go || fail "internal/app must compose httpapi.Server"

echo "backend structure ok"
