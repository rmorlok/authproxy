---
title: Install AuthProxy with Helm
description: Install AuthProxy on Kubernetes with the official Helm chart and production dependency settings.
---

The chart is published as an OCI artifact at
`oci://ghcr.io/rmorlok/charts/authproxy`. Chart versions and application image
versions are independent.

## Prerequisites

- a Kubernetes cluster and `kubectl` access;
- Helm 3;
- PostgreSQL and Redis endpoints for a durable install;
- an ingress controller if browser or OAuth endpoints are exposed; and
- operator-created Secrets for database credentials, JWT signing, and
  application-level encryption.

## 1. Inspect the chart

Choose an explicit chart version:

```bash
helm show chart oci://ghcr.io/rmorlok/charts/authproxy --version 0.1.0
helm show values oci://ghcr.io/rmorlok/charts/authproxy --version 0.1.0 > values.reference.yaml
```

The full values schema also lives beside the chart at
[`deploy/charts/authproxy/values.yaml`](https://github.com/rmorlok/authproxy/blob/main/deploy/charts/authproxy/values.yaml).

## 2. Create key material and Secrets

The chart references existing Secrets; it does not generate sensitive
material.

```bash
workdir="$(mktemp -d)"
openssl genrsa -out "$workdir/system" 3072
openssl rsa -in "$workdir/system" -pubout -out "$workdir/system.pub"
openssl rand 32 > "$workdir/global_aes.key"

kubectl create namespace authproxy

kubectl -n authproxy create secret generic authproxy-jwt \
  --from-file=system="$workdir/system" \
  --from-file=system.pub="$workdir/system.pub"

kubectl -n authproxy create secret generic authproxy-encryption \
  --from-file=global_aes.key="$workdir/global_aes.key"

kubectl -n authproxy create secret generic authproxy-db \
  --from-literal=AUTHPROXY_DB_PASSWORD='<database-password>'

kubectl -n authproxy create secret generic authproxy-redis \
  --from-literal=AUTHPROXY_REDIS_PASSWORD='<redis-password>'
```

Handle the temporary directory as secret material and remove it after storing
the keys in your approved secret system. For production, prefer a managed KMS
or secret provider and a repeatable rotation process; see
[encryption](/security/encryption/).

## 3. Create a values file

This example shows the essential connectivity and secret references. Replace
hosts and URLs for your environment:

```yaml
image:
  tag: "1.2.3"

database:
  provider: postgres
  host: postgres.example.internal
  database: authproxy
  user: authproxy
  sslmode: verify-full
  existingSecret: authproxy-db

redis:
  provider: redis
  address: redis.example.internal:6379
  existingSecret: authproxy-redis

jwt:
  existingSecret: authproxy-jwt

encryptionKeys:
  existingSecret: authproxy-encryption

hostApplication:
  initiateSessionUrl: https://app.example.com/integrations/login

services:
  public:
    baseUrl: https://connect.example.com
    static:
      enabled: true
  adminApi:
    ui:
      enabled: true
      baseUrl: https://authproxy-admin.example.internal
      initiateSessionUrl: https://app.example.com/admin/authproxy/login
    static:
      enabled: true
```

Configure `ingress`, blob storage, actors, resources, and telemetry for the
target platform. Do not expose Admin or API endpoints by default.

## 4. Install and verify

```bash
helm upgrade --install authproxy \
  oci://ghcr.io/rmorlok/charts/authproxy \
  --version 0.1.0 \
  --namespace authproxy \
  --values values.yaml \
  --wait \
  --timeout 10m

kubectl -n authproxy get deploy,pod,service,ingress
kubectl -n authproxy rollout status deployment/authproxy
```

Verify `/ping` through each endpoint that should be reachable, then exercise a
real host-signed session and a test connector before admitting users.

## Values groups

| Values section | Purpose |
|---|---|
| `image` | Repository, immutable tag, and pull policy |
| `services.*` | Enable API, Admin, public, and worker services |
| `ingress` | Host, path, TLS, and service routing |
| `database`, `redis` | Primary persistence and distributed state |
| `blobStorage`, `s3` | Optional full request/response payload storage |
| `jwt`, `actors`, `encryptionKeys` | Secret mounts and key paths |
| `hostApplication` | Browser-session redirect back to the embedding app |
| `connectors` | Seeded connector definitions and identifying labels |
| `appMetrics` | Request events, resource metrics, and optional full recording |
| `config` | Advanced free-form overlay, including telemetry settings not yet typed by the chart |

Use the typed sections where available and reserve `config` for settings the
chart does not yet expose.

## Upgrade guidance

- Pin the chart and image separately; do not rely on `main` or `latest` in a
  controlled environment.
- Render changes with `helm template` and review Secret references before
  applying.
- Back up persistent stores before schema-changing upgrades.
- Keep old JWT and encryption material available for the documented transition
  window; those rotations have different lifecycle requirements.
- Verify worker queues and OAuth callbacks after rollout.

The source chart's package README remains at
[`deploy/charts/authproxy/README.md`](https://github.com/rmorlok/authproxy/blob/main/deploy/charts/authproxy/README.md).
