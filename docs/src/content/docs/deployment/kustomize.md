---
title: Demo Kustomize deployments
---

The Kustomize tree under `deploy/kustomize/authproxy-demo` deploys AuthProxy's
hosted demo and per-pull-request demo environments. It is not the current
customer-facing production package; use the [Helm chart](/deployment/helm/) for general
installations.

## Layout

```text
deploy/kustomize/authproxy-demo/
├── base/              # AuthProxy, Demo Shell, and fake OAuth provider
└── overlays/
    ├── demo/          # Persistent demo.authproxy.net environment
    └── dev/           # Disposable per-branch environment
```

The persistent demo adds PostgreSQL, Redis, MinIO, Grafana, Prometheus, Tempo,
Loki, and an OpenTelemetry Collector. The dev overlay uses SQLite, in-process
miniredis, and filesystem blob storage so it can be recreated cheaply.

Never use the dev storage profile where connections, sessions, queues, request
events, or blobs must survive a pod replacement.

## Render before applying

```bash
kubectl kustomize deploy/kustomize/authproxy-demo/overlays/demo > /tmp/authproxy-demo.yaml
kubectl kustomize deploy/kustomize/authproxy-demo/overlays/dev > /tmp/authproxy-dev.yaml
```

Inspect image tags, hostnames, storage providers, Secret references, and
Ingress rules in the rendered output.

## Secret contract

The workflows create or preserve Secrets outside the Kustomize apply. After
the overlay's `namePrefix`, it expects names for:

- JWT signing keys;
- the global encryption key;
- configured actor keys;
- the Demo Shell signing key; and
- database, Redis, and MinIO credentials in the persistent demo.

Both deployment workflows run the overlay's `seed` Job after rollout and
before smoke tests. The job configures the disposable OAuth provider, then
uses `ap apply` to reconcile namespaces, actors, and connectors from the
ConfigMap's `resources.yaml`. The CLI and server are built from the same
revision. The manual `Seed Demo` workflow reruns that job when needed.

AuthProxy resources are standard multi-document manifests. `seed.yaml` contains
only external provider setup; the server configuration has an empty connector
loader. Existing demo resources are adopted by namespace/name on the first
apply, with later deploys using last-applied history. Provisioning failures fail
the deployment. Prune is disabled, so removing a manifest does not delete or
archive resources.

Remote smoke tests verify the applied demo catalog and create isolated,
uniquely named connector copies through the API for each test run. API creation
is retained there to exercise provisioning and avoid changing the shared demo
catalog. These temporary connectors are archived during cleanup; they are never
loaded from server configuration.

## Hosted deployment behavior

`Deploy Demo` pins AuthProxy and demo images to the selected commit, applies the
persistent overlay, waits for workloads, and smoke-tests the shell, UIs,
Grafana data sources, and fake OAuth provider. `Deploy Dev` creates an isolated
namespace only for same-repository pull requests carrying the `deploy:demo`
label and tears it down when the pull request closes.

See the [source package README](https://github.com/rmorlok/authproxy/blob/main/deploy/kustomize/authproxy-demo/README.md)
and [EKS runbook](/deployment/eks-runbook/) when maintaining those project-owned
environments.

OAuth manifests explicitly contain disposable provider client secrets. The CLI
resubmits explicit secrets without comparing them, so each deploy publishes a
new OAuth connector generation while retaining the connector ID. Other unchanged
resources converge to `unchanged`. This follows the CLI's secret-handling
contract; secrets and comparison hashes are never stored in apply history.
