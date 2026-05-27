#!/usr/bin/env bash
# Generate the demo-shell admin keypair used by docker-compose.yaml.
# Idempotent — skips generation if both files already exist.
set -euo pipefail

cd "$(dirname "$0")"
mkdir -p keys

if [[ -f keys/demo-shell && -f keys/demo-shell.pub ]]; then
  echo "keys/demo-shell{,.pub} already exist — skipping (delete them to regenerate)."
  exit 0
fi

openssl genrsa -out keys/demo-shell 2048
openssl rsa -in keys/demo-shell -pubout -out keys/demo-shell.pub
chmod 600 keys/demo-shell

echo "Generated keys/demo-shell + keys/demo-shell.pub. Run 'docker compose up' next."
