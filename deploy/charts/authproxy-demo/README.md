# authproxy-demo Helm chart

Umbrella chart for the AuthProxy demo environment. Composes:

| Subchart / workload     | Role                                                        |
|-------------------------|-------------------------------------------------------------|
| `authproxy` (local)     | The AuthProxy server itself (the customer chart)            |
| `postgresql` (bitnami)  | Backing DB                                                  |
| `redis` (bitnami)       | Sessions + tasks + rate-limit                               |
| `minio` (bitnami)       | Request-log blob storage                                    |
| `demo-shell` (inline)   | SSO stand-in host (signs JWTs for demo actors)              |
| `go-oauth2-server` (inline) | Connector target for the demo OAuth flow                |

Single Ingress with sub-path routing fronts everything behind one
hostname. TLS via cert-manager.

This is the **demo** chart — never install it for a customer. The
production install path is `deploy/charts/authproxy` directly.

## Pre-requisites

1. EKS (or any K8s) cluster with the bootstrap layer installed
   (`deploy/charts/bootstrap`): ingress-nginx, cert-manager, external-dns,
   a `letsencrypt-prod` ClusterIssuer.
2. A DNS A record pointing at the cluster's NLB. external-dns will
   create this automatically once the Ingress is applied — see the
   bootstrap chart README for `zoneIdFilters` setup.
3. Helm 3.16+ with subchart fetching enabled.

## Install

```bash
cd deploy/charts/authproxy-demo
helm dependency update      # pulls authproxy/* and the bitnami charts

helm upgrade --install demo . \
  --namespace demo --create-namespace \
  --wait --timeout 10m \
  --set "global.hostname=demo.authproxy.net"
```

First install takes ~3-5 min once the bitnami subcharts finish their
PVC provisioning + initial schema. The NOTES print a verification
checklist.

## What gets auto-generated

When `secrets.autoGenerate: true` (the default), the chart materializes
the following Secrets on first install:

| Secret                  | Contents                                          |
|-------------------------|---------------------------------------------------|
| `<release>-jwt`         | RSA keypair for AuthProxy JWT signing             |
| `<release>-encryption`  | 32-byte global AES key                            |
| `<release>-demo-shell-key` | demo-shell admin RSA keypair (`private` + `<name>.pub`) |
| `<release>-actors`      | `demo-shell.pub` for AuthProxy's actors keys_path |
| `<release>-db`          | Postgres password                                 |
| `<release>-redis-creds` | Redis password                                    |
| `<release>-minio-creds` | MinIO root password                               |

On `helm upgrade`, the templates `lookup` the existing Secrets and
reuse their values — sessions persist, no rotation surprises. To
rotate, `kubectl delete secret <release>-...` then `helm upgrade` will
mint fresh material.

## Sub-path routing

The umbrella Ingress maps host paths to backend services:

| Path           | Backend                                     |
|----------------|---------------------------------------------|
| `/`            | demo-shell                                  |
| `/admin`       | AuthProxy admin-api (serves admin UI)       |
| `/admin-api`   | AuthProxy admin-api (JSON API)              |
| `/marketplace` | AuthProxy public (serves marketplace UI)    |
| `/api`         | AuthProxy api                               |
| `/public`      | AuthProxy public (OAuth callbacks, etc.)    |
| `/oauth2`      | go-oauth2-server                            |

(Paths are `pathType: Prefix`; order in the Ingress is significant only
for `pathType: Exact` matches.)

## Smoke test

After install, open `https://<hostname>` — the demo-shell page loads.
Pick **demo-admin** + **Admin UI** → submit → you should land in the
admin UI as `demo-admin`. Pick **fresh-user** + **Marketplace UI** →
empty marketplace, no connections.

## Uninstall

```bash
helm uninstall demo --namespace demo
kubectl delete ns demo
```

Persistent volumes claimed by postgres / redis / minio survive the
helm release unless you also `kubectl delete pvc -n demo --all`.
