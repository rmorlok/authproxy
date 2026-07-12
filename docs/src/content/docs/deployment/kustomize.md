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

Seeding is intentionally separate from deployment. The `Seed Demo` workflow
renders the overlay's `seed` directory and runs a one-shot Job to create demo
actors, fake OAuth users, and example connectors.

## Hosted deployment behavior

`Deploy Demo` pins AuthProxy and demo images to the selected commit, applies the
persistent overlay, waits for workloads, and smoke-tests the shell, UIs,
Grafana data sources, and fake OAuth provider. `Deploy Dev` creates an isolated
namespace only for same-repository pull requests carrying the `deploy:demo`
label and tears it down when the pull request closes.

See the [source package README](https://github.com/rmorlok/authproxy/blob/main/deploy/kustomize/authproxy-demo/README.md)
and [EKS runbook](/deployment/eks-runbook/) when maintaining those project-owned
environments.
