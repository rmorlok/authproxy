---
title: Development quick start
description: Clone AuthProxy and start a complete local stack with Docker Compose.
---

This is the shortest path from a fresh checkout to a running build of the
current source.

## Prerequisites

- [Git](https://git-scm.com/)
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) or Docker
  Engine with Compose

## Start AuthProxy

```bash
git clone https://github.com/rmorlok/authproxy.git
cd authproxy
docker compose --profile server up --build -d
```

The build compiles the Marketplace and Admin UIs into the AuthProxy image. The
Compose stack starts:

| Component | Address |
|---|---|
| Public service and embedded Marketplace | `https://localhost:8080` |
| API | `http://localhost:8081` |
| Admin API and embedded Admin UI | `https://localhost:8082` |
| Worker health service | `http://localhost:8083` |
| Postgres | `localhost:5432` |
| Redis | `localhost:6379` |
| MinIO API / console | `localhost:9000` / `localhost:9001` |
| ClickHouse HTTP | `localhost:8123` |

Confirm that AuthProxy and its dependencies are ready:

```bash
curl http://localhost:8081/ping
```

The public and Admin services use an automatically generated, self-signed
development certificate. A browser warning is expected until you trust that
certificate.

## Stop or reset

Stop containers while keeping data volumes:

```bash
docker compose --profile server down
```

Remove all local AuthProxy containers and data volumes:

```bash
./scripts/teardown-docker.sh
```

## Next steps

- [Run services and UIs from source](/development/local-development/)
- [Understand the codebase](/development/codebase/)
- [Try the complete hosted demo](/getting-started/demo/)
