#!/bin/bash

# Generate Swagger documentation for AuthProxy
# This script generates the swagger.json and swagger.yaml files from Go annotations
#
# Prerequisites:
#   go install github.com/swaggo/swag/cmd/swag@latest
#
# Usage:
#   ./scripts/generate-swagger.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "Generating Swagger documentation..."

# Find swag command - check in PATH first, then GOPATH/bin
SWAG_CMD=""
if command -v swag &> /dev/null; then
    SWAG_CMD="swag"
elif [ -x "$(go env GOPATH)/bin/swag" ]; then
    SWAG_CMD="$(go env GOPATH)/bin/swag"
else
    echo "Error: swag command not found. Please install it with:"
    echo "  go install github.com/swaggo/swag/cmd/swag@latest"
    exit 1
fi

echo "Using swag command: $SWAG_CMD"

# Generate swagger docs
# -g: main API documentation file
# -o: output directory
# --parseDependency: parse external dependencies
# --parseInternal: parse internal packages
$SWAG_CMD init \
    -g internal/docs/doc.go \
    -o internal/docs \
    --parseDependency \
    --parseInternal \
    --dir ./

echo "Swagger documentation generated successfully!"
echo "Output files:"
echo "  - internal/docs/docs.go"
echo "  - internal/docs/swagger.json"
echo "  - internal/docs/swagger.yaml"
echo ""
echo "Access the Swagger UI at:"
echo "  - Admin API: http://localhost:8082/swagger/index.html"
echo "  - API: http://localhost:8081/swagger/index.html"
