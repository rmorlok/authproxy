---
title: Container images and registries
---

AuthProxy publishes multi-architecture images to GitHub Container Registry.

## Server image

```text
ghcr.io/rmorlok/authproxy:<tag>
```

The image contains the Go server and embedded Marketplace and Admin UI assets.
Its default command starts all services with `dev_config/docker.yaml`; a real
deployment should mount or generate its own configuration and Secrets.

Published tag forms include:

| Tag | Meaning |
|---|---|
| `X.Y.Z` | Exact application release |
| `X.Y` | Moving tag for the latest patch in a release line |
| `latest` | Latest tagged release |
| `main` | Latest successful build from the default branch |
| `sha-<short>` | A specific commit build |

Pin an exact release or image digest in controlled environments. `main`,
`latest`, and minor-line tags move over time.

```bash
docker pull ghcr.io/rmorlok/authproxy:1.2.3
docker inspect ghcr.io/rmorlok/authproxy:1.2.3
```

## Helm chart

The chart is a separate OCI artifact:

```text
oci://ghcr.io/rmorlok/charts/authproxy
```

Tags named `chart-vX.Y.Z` in Git publish chart version `X.Y.Z`. The chart's
version is independent of the server image version, and `image.tag` can be
overridden per installation.

## Demo-only images

The repository also publishes `authproxy-demo-shell`, `authproxy-demo-seed`,
and `authproxy-demo-grafana`. They support the public demo and are not required
for a normal AuthProxy deployment.

## Build locally

From the repository root:

```bash
docker build -t authproxy:local .
```

The build compiles both UIs before compiling the server so the resulting image
matches the production artifact shape.
