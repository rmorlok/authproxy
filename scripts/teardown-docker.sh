#!/bin/bash

# Tear down the Docker development environment for AuthProxy
# This stops all containers, removes volumes, and cleans up the network
# so the next startup will create everything from fresh.
#
# Usage:
#   ./scripts/teardown-docker.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "Tearing down AuthProxy Docker environment..."

# Stop and remove all containers and volumes managed by docker-compose
# Include all profiles to ensure everything is cleaned up
echo "Stopping docker compose services and removing volumes..."
docker compose --profile server --profile tools down -v 2>/dev/null || true

# Also clean up any manually-started containers from the manual setup instructions
MANUAL_CONTAINERS="redis-server postgres-server clickhouse-server minio redisinsight asynqmon"
for container in $MANUAL_CONTAINERS; do
    if docker ps -a --format '{{.Names}}' | grep -q "^${container}$"; then
        echo "Removing manually-started container: ${container}"
        docker rm -f "$container" 2>/dev/null || true
    fi
done

# Remove manually-created volumes (from docker run -v name:/path)
MANUAL_VOLUMES="redisinsight"
for volume in $MANUAL_VOLUMES; do
    if docker volume ls --format '{{.Name}}' | grep -q "^${volume}$"; then
        echo "Removing volume: ${volume}"
        docker volume rm "$volume" 2>/dev/null || true
    fi
done

# Remove the authproxy network if it was created manually
if docker network ls --format '{{.Name}}' | grep -q "^authproxy$"; then
    echo "Removing manually-created 'authproxy' network..."
    docker network rm authproxy 2>/dev/null || true
fi

echo ""
echo "Teardown complete. All containers, volumes, and networks have been removed."
echo "Run 'docker compose up -d' to start fresh."
