# AuthProxy Helm Chart

[AuthProxy](https://github.com/rmorlok/authproxy) is an open-source, embeddable
integration platform-as-a-service. It manages the connection lifecycle to
third-party systems and proxies authenticated requests for an embedding host
application.

This chart deploys the AuthProxy server (`api`, `admin-api`, `public`, and
`worker` services) as a single Deployment with a configurable subset of
services enabled.

## TL;DR

```bash
# 1. Create the Secrets the chart references (it deliberately never generates them).
kubectl create secret generic authproxy-jwt \
  --from-file=system=/path/to/jwt-private \
  --from-file=system.pub=/path/to/jwt-public

kubectl create secret generic authproxy-encryption \
  --from-file=global_aes.key=/path/to/global_aes.key

# 2. Install from the GHCR OCI registry.
helm install authproxy oci://ghcr.io/rmorlok/charts/authproxy \
  --version <chart-version> \
  --set database.provider=postgres \
  --set database.host=postgres.example.com \
  --set database.existingSecret=authproxy-db \
  --set redis.address=redis.example.com:6379 \
  --set jwt.existingSecret=authproxy-jwt \
  --set encryptionKeys.existingSecret=authproxy-encryption \
  --set hostApplication.initiateSessionUrl=https://app.example.com/login
```

## Installing from the OCI registry

Charts are published to `oci://ghcr.io/rmorlok/charts/authproxy` on each
`chart-vX.Y.Z` tag. The chart version is independent of the AuthProxy
application version (`image.tag`).

```bash
# Inspect available versions:
helm show chart oci://ghcr.io/rmorlok/charts/authproxy --version 0.1.0

# Pull a tarball locally:
helm pull oci://ghcr.io/rmorlok/charts/authproxy --version 0.1.0

# Install:
helm install authproxy oci://ghcr.io/rmorlok/charts/authproxy \
  --version 0.1.0 \
  -f my-values.yaml
```

## Values reference

The chart exposes typed values for the common connectivity blocks. See
[`values.yaml`](values.yaml) for the full surface; the section headings:

| Section          | Purpose                                                              |
|------------------|----------------------------------------------------------------------|
| `image`          | Container image repository, tag, pull policy                         |
| `services.*`     | Per-service enable toggles (`api`, `adminApi`, `public`, `worker`)   |
| `autoscaling`    | Optional autoscaling/v2 HPA for the chart release                    |
| `ports`          | Container port assignments                                            |
| `ingress`        | Single Ingress with path → port-name routing                          |
| `database`       | Postgres or SQLite connectivity (+ `existingSecret` for credentials)  |
| `redis`          | Redis connectivity (+ optional `existingSecret`)                      |
| `s3`             | Legacy S3-compatible blob storage inputs                              |
| `blobStorage`    | Request-log blob storage provider, including local filesystem mode    |
| `jwt`            | Secret mount + key paths for JWT signing material                     |
| `encryptionKeys` | Secret mount + path for the global AES key                            |
| `actors`         | Secret mount + ACL permissions for admin actor keypairs               |
| `hostApplication`| URL the marketplace UI redirects to for login                         |
| `connectors`     | Inline connector definitions + identifying labels                     |
| `appMetrics`     | App metrics + request-event storage (defaults to shared main DB)      |
| `config`         | Free-form overlay merged into the rendered AuthProxy YAML             |

### Secrets the chart expects you to create

The chart **never generates** sensitive material — instead, it references
Secrets by name. You provision them once and pass the names via `existingSecret`:

| Values key                          | Secret keys expected                                  |
|-------------------------------------|-------------------------------------------------------|
| `jwt.existingSecret`                | `system` (private), `system.pub` (public)             |
| `encryptionKeys.existingSecret`     | `global_aes.key` (32 raw bytes)                       |
| `actors.existingSecret`             | `<name>` + `<name>.pub` pairs for each admin actor    |
| `database.existingSecret` *(optional)* | `AUTHPROXY_DB_PASSWORD`                            |
| `redis.existingSecret` *(optional)* | `AUTHPROXY_REDIS_PASSWORD`                            |
| `s3.existingSecret` *(optional)*    | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`          |

For disposable environments, set `blobStorage.provider=filesystem`; the chart
mounts an `emptyDir` at `blobStorage.filesystem.path` by default. Persistent
deployments should continue to use S3-compatible storage.

## Independent Service Scaling

The chart renders one Deployment for the enabled service set. For production or
load-test environments that need independent scale curves, install the chart
multiple times with different `services` toggles:

```bash
helm upgrade --install authproxy-api oci://ghcr.io/rmorlok/charts/authproxy \
  -f common-values.yaml \
  --set services.api.enabled=true \
  --set services.adminApi.enabled=false \
  --set services.public.enabled=false \
  --set services.worker.enabled=false \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=16 \
  --set autoscaling.targetCPUUtilizationPercentage=70

helm upgrade --install authproxy-worker oci://ghcr.io/rmorlok/charts/authproxy \
  -f common-values.yaml \
  --set services.api.enabled=false \
  --set services.adminApi.enabled=false \
  --set services.public.enabled=false \
  --set services.worker.enabled=true \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=1 \
  --set autoscaling.maxReplicas=8
```

`autoscaling.enabled=false` preserves the legacy `replicaCount` behavior. When
autoscaling is enabled, the Deployment omits `spec.replicas` and the
HorizontalPodAutoscaler owns the desired replica count.

### Custom Metrics

`autoscaling.metrics` accepts raw Kubernetes `autoscaling/v2` metric entries.
For example, a worker release can scale on queue depth if your cluster exposes
an adapter metric such as `authproxy_asynq_queue_size`:

```yaml
autoscaling:
  enabled: true
  minReplicas: 1
  maxReplicas: 8
  targetCPUUtilizationPercentage: 75
  metrics:
    - type: Pods
      pods:
        metric:
          name: authproxy_asynq_queue_size
        target:
          type: AverageValue
          averageValue: "100"
```

## Compatibility

| Chart version | App version (`appVersion`) |
|---------------|----------------------------|
| 0.1.x         | `main`                     |

The chart's `appVersion` is automatically pinned to the latest `vX.Y.Z` app tag
at chart-release time. Override per-deploy with `--set image.tag=<tag>`.

## Development

For chart-on-source-tree work (running CI locally, iterating on templates),
see [`AGENTS.md`](../../AGENTS.md) at the repo root and the chart-testing
CI workflow under `.github/workflows/helm-chart-ci.yml`.
