#!/bin/bash

# Generate Swagger documentation for AuthProxy
# This script generates the swagger.json and swagger.yaml files from Go annotations
#
# Prerequisites:
#   go install github.com/swaggo/swag/cmd/swag@latest
#
# Usage:
#   ./scripts/generate-swagger.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "Generating Swagger documentation..."

generate_api_swagger() {
    # Generate swagger docs
    # -g: main API documentation file
    # -o: output directory
    # --parseDependency: parse external dependencies
    # --parseInternal: parse internal packages
    go run github.com/swaggo/swag/cmd/swag init \
        -g internal/service/api/swagger/definition.go \
        -o internal/service/api/swagger \
        --parseDependency \
        --parseInternal \
        --instanceName Api \
        --dir ./

    mv internal/service/api/swagger/Api_docs.go internal/service/api/swagger/docs.go
    mv internal/service/api/swagger/Api_swagger.json internal/service/api/swagger/docs.json
    mv internal/service/api/swagger/Api_swagger.yaml internal/service/api/swagger/docs.yaml
}

generate_admin_api_swagger() {
    go run github.com/swaggo/swag/cmd/swag init \
        -g internal/service/admin_api/swagger/definition.go \
        -o internal/service/admin_api/swagger \
        --parseDependency \
        --parseInternal \
        --instanceName admin_api \
        --dir ./

    mv internal/service/admin_api/swagger/admin_api_docs.go internal/service/admin_api/swagger/docs.go
    mv internal/service/admin_api/swagger/admin_api_swagger.json internal/service/admin_api/swagger/docs.json
    mv internal/service/admin_api/swagger/admin_api_swagger.yaml internal/service/admin_api/swagger/docs.yaml
}

generate_api_swagger &
api_pid=$!

generate_admin_api_swagger &
admin_api_pid=$!

status=0
wait "$api_pid" || status=$?
wait "$admin_api_pid" || status=$?

if [[ "$status" -ne 0 ]]; then
    exit "$status"
fi

echo "Swagger documentation generated successfully!"
echo "Output files:"
echo "  - internal/service/api/swagger/docs.go"
echo "  - internal/service/api/swagger/docs.json"
echo "  - internal/service/api/swagger/docs.yaml"
echo "  - internal/service/admin_api/swagger/docs.go"
echo "  - internal/service/admin_api/swagger/docs.json"
echo "  - internal/service/admin_api/swagger/docs.yaml"
echo ""
echo "Access the Swagger UI at:"
echo "  - Admin API: http://localhost:8082/swagger/index.html"
echo "  - API: http://localhost:8081/swagger/index.html"
