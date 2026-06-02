# authproxy-demo Helm chart

Umbrella chart for the AuthProxy demo environment. Composes:

| Subchart / workload     | Role                                                        |
|-------------------------|-------------------------------------------------------------|
| `authproxy` (local)     | The AuthProxy server itself (the customer chart)            |
| `postgresql` (bitnami)  | Backing DB                                                  |
| `redis` (bitnami)       | Sessions + tasks + rate-limit                               |
| `minio` (bitnami)       | Request-log blob storage                                    |
| `grafana` (upstream)    | App-metrics datasource, variables, and sample dashboards    |
| `demo-shell` (inline)   | SSO stand-in host (signs JWTs for demo actors)              |
| `go-oauth2-server` (inline) | Connector target for the demo OAuth flow                |

Ingress fronts the demo shell and test OAuth provider on the apex host,
with dedicated subdomains for the embedded marketplace and admin SPAs.
TLS is issued by cert-manager.

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
helm dependency update      # pulls authproxy/*, Grafana, and the bitnami charts

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
| `<release>-actors`         | `demo-shell.pub` plus one `<actor>.pub` entry for each demo actor, all matching the demo shell public key |
| `authproxy-grafana-jwt`    | `jwt` (Grafana datasource bearer token, can be generated after `<release>-encryption`) |

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
  --from-file=demo-shell.pub="$work/demo-shell.pub" \
  --from-file=demo-admin.pub="$work/demo-shell.pub" \
  --from-file=demo-user.pub="$work/demo-shell.pub" \
  --from-file=fresh-user.pub="$work/demo-shell.pub" \
  --from-file=grafana.pub="$work/demo-shell.pub"

# Metrics plus request-event metadata tables. Database-backed actors
# without their own self-signing key are verified with the global AES
# key, and the Grafana preset only restricts the token further. The
# actor identified in the token must still have matching normal
# AuthProxy permissions.
ap sign-jwt \
  --actorId grafana \
  --apis api,admin-api \
  --grafana-preset logs \
  --secretKeyPath "$work/global_aes.key" \
  > "$work/grafana.jwt"
kubectl -n "$NS" create secret generic authproxy-grafana-jwt \
  --from-file=jwt="$work/grafana.jwt"
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
| `grafana`    | datasource | Identity used by the provisioned Grafana datasource token |

Idempotent: the seed binary GETs each actor by external_id and skips
creation when AuthProxy returns 200. Re-running `helm upgrade` is a
no-op once the state matches. Toggle off with `--set seed.enabled=false`
or extend via `seed.actors[*]` to seed customer-specific identities
alongside the defaults.

> **Currently NOT seeded:** per-actor pre-existing connections + the
> demo connector itself. The connector pattern needs a dedicated
> connector-management admin API surface that isn't fully there yet;
> tracked as a follow-up to A11.

## Routing

The umbrella Ingress maps public hosts and paths to backend services:

| Host / path                | Backend                                  |
|----------------------------|------------------------------------------|
| `<hostname>/`              | demo-shell                               |
| `<hostname>/oauth2`        | go-oauth2-server                         |
| `<hostname>/grafana`       | Grafana dashboards                       |
| `marketplace.<hostname>/`  | AuthProxy public (marketplace UI + API)  |
| `admin.<hostname>/`        | AuthProxy admin-api (admin UI + API)     |

The OAuth test provider is path-routed because it is not a SPA. The
marketplace and admin UIs use subdomains so their root-relative Vite
assets resolve correctly.

## Smoke test

After install, open `https://<hostname>` — the demo-shell page loads.
Pick **demo-admin** + **Admin UI** → submit → you should land in the
admin UI as `demo-admin`. Pick **fresh-user** + **Marketplace UI** →
empty marketplace, no connections.

Grafana is available at `https://<hostname>/grafana` when
`grafana.enabled=true` (the default). The chart provisions:

- `AuthProxy` datasource (`uid: authproxy-app-metrics`) pointed at the
  in-cluster AuthProxy API service.
- `AuthProxy App Metrics` dashboard with resource counts, connection
  states, request volume/errors/duration, rate-limit attribution, and
  request-event metadata.
- Dashboard variables for namespaces, connectors, and connections.

The datasource JWT comes from the `authproxy-grafana-jwt` Secret and is
injected into Grafana as `AUTHPROXY_GRAFANA_JWT`; override
`grafana.envValueFrom` if you use a differently named Secret.

By default the chart asks Grafana to install the
`rmorlok-authproxy-datasource` plugin by id. If you are testing a plugin
release artifact before it is available in the Grafana catalog, override
`grafana.plugins[0]` with Grafana's URL install format:
`https://.../rmorlok-authproxy-datasource.zip;rmorlok-authproxy-datasource`.
The `deploy-demo.yml` workflow reads the same override from the optional
`GRAFANA_AUTHPROXY_PLUGIN_INSTALL` repository variable. Until the plugin
is available from the Grafana catalog, the workflow disables Grafana when
that variable is unset so the main demo rollout is not blocked by plugin
installation.

## Uninstall

```bash
helm uninstall demo --namespace demo
kubectl delete ns demo
```

Persistent volumes claimed by postgres / redis / minio survive the
helm release unless you also `kubectl delete pvc -n demo --all`.
