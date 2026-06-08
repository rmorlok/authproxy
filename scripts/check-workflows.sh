#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

go test ./internal/core -run 'TestDisconnectConnectionWorkflowV1|TestRegisterDisconnectConnectionWorkflowV1' -count=1
