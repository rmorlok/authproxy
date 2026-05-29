#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

cd "$ROOT_DIR"

fail() {
  printf "[schema-layout] ERROR: %s\n" "$*" >&2
  exit 1
}

if rg -n '"github\.com/rmorlok/authproxy/internal/schema/api' internal/schema/resources; then
  fail "resource schema packages must not import internal/schema/api"
fi

if rg -n '^type[[:space:]]+[A-Za-z0-9_]*(RequestJson|ResponseJson)[[:space:]]+struct' internal/routes; then
  fail "route-local public API request/response DTOs must live in internal/schema/api"
fi

if rg -n '^type[[:space:]]+Swagger[A-Za-z0-9_]*[[:space:]]+(struct|=)' internal/routes; then
  fail "Swagger-only route models must live in internal/schema/api/openapi"
fi

printf "[schema-layout] Schema layout checks passed\n"
