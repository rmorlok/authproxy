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

## What gets auto-generated vs. operator-provided

When `secrets.autoGenerate: true` (the default), the chart materializes
the following Secrets on first install via sprig template helpers
(`randAlphaNum`):

| Secret                  | Contents                |
|-------------------------|-------------------------|
| `<release>-db`          | Postgres password       |
| `<release>-redis-creds` | Redis password          |
| `<release>-minio-creds` | MinIO root password     |

On `helm upgrade`, the templates `lookup` the existing Secrets and
reuse their values — passwords persist across upgrades. To rotate,
`kubectl delete secret <release>-...` then `helm upgrade`.

**Operator-provided** (sprig can't derive a public key from a private
key, so these need real openssl). Create them in the release namespace
**before `helm install`**:

| Secret                     | Keys expected                                  |
|----------------------------|------------------------------------------------|
| `<release>-jwt`            | `system` (RSA private), `system.pub`           |
| `<release>-encryption`     | `global_aes.key` (32 raw bytes)                |
| `<release>-demo-shell-key` | `private` (RSA private), `demo-shell.pub`      |
| `<release>-actors`         | `demo-shell.pub` (matching the above)          |

A reference one-shot generator:

```bash
RELEASE=demo
NS=demo
work=$(mktemp -d)
openssl genrsa -out "$work/system" 2048
openssl rsa -in "$work/system" -pubout -out "$work/system.pub"
openssl genrsa -out "$work/demo-shell" 2048
openssl rsa -in "$work/demo-shell" -pubout -out "$work/demo-shell.pub"
openssl rand -out "$work/global_aes.key" 32

kubectl create ns "$NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "$NS" create secret generic "$RELEASE-jwt" \
  --from-file=system="$work/system" \
  --from-file=system.pub="$work/system.pub"
kubectl -n "$NS" create secret generic "$RELEASE-encryption" \
  --from-file=global_aes.key="$work/global_aes.key"
kubectl -n "$NS" create secret generic "$RELEASE-demo-shell-key" \
  --from-file=private="$work/demo-shell" \
  --from-file=demo-shell.pub="$work/demo-shell.pub"
kubectl -n "$NS" create secret generic "$RELEASE-actors" \
  --from-file=demo-shell.pub="$work/demo-shell.pub"
```

A Helm pre-install hook Job that generates the keypair Secrets is a
reasonable follow-up — would unify the operator UX with the
auto-generated random material above. Tracked separately.

## Demo actors (seeded on every install/upgrade)

A `post-install` + `post-upgrade` Helm hook Job runs the
`authproxy-demo-seed` image, which calls the AuthProxy admin API to
create the three demo identities the shell expects:

| External ID  | Role     | Notes                                            |
|--------------|----------|--------------------------------------------------|
| `demo-admin` | admin    | Lands in the admin UI                            |
| `demo-user`  | user     | Lands in the marketplace                         |
| `fresh-user` | user     | Lands in the marketplace with no connections     |

Idempotent: the seed binary GETs each actor by external_id and skips
creation when AuthProxy returns 200. Re-running `helm upgrade` is a
no-op once the state matches. Toggle off with `--set seed.enabled=false`
or extend via `seed.actors[*]` to seed customer-specific identities
alongside the defaults.

> **Currently NOT seeded:** per-actor pre-existing connections + the
> demo connector itself. The connector pattern needs a dedicated
> connector-management admin API surface that isn't fully there yet;
> tracked as a follow-up to A11.

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
