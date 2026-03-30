#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

cd "$ROOT_DIR"

say() {
  printf "[preflight] %s\n" "$*"
}

fail() {
  printf "[preflight] ERROR: %s\n" "$*" >&2
  exit 1
}

SWAGGER_FILES=(
  "internal/service/api/swagger/docs.go"
  "internal/service/api/swagger/docs.json"
  "internal/service/api/swagger/docs.yaml"
  "internal/service/admin_api/swagger/docs.go"
  "internal/service/admin_api/swagger/docs.json"
  "internal/service/admin_api/swagger/docs.yaml"
)

say "Generating Swagger docs"
"$ROOT_DIR/scripts/generate-swagger.sh" >/dev/null

if ! git diff --name-only -- "${SWAGGER_FILES[@]}" | grep -q .; then
  say "Swagger docs are up to date"
else
  fail "Swagger docs changed. Run ./scripts/generate-swagger.sh and commit the updated docs."
fi

say "Checking integration_tests module (go list -mod=readonly)"
(
  cd "$ROOT_DIR/integration_tests"
  go list -mod=readonly ./... >/dev/null
)

say "Preflight passed"
