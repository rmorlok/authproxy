# AuthProxy Load-Test Harness

This directory contains the Kubernetes harness for the AuthProxy load-test
project tracked by #711 and #717. It is intentionally split from product
optimizations: this first slice gives us repeatable environment setup, smoke
traffic, and artifact capture. The large-state seeder, proxy-QPS scenarios, and
background-job suites build on this foundation in follow-up issues.

## Prerequisites

- `kubectl` pointed at the target cluster.
- `helm` for installing AuthProxy from the local chart.
- `openssl` for disposable JWT, actor, and AES keys.
- Metrics Server in the cluster when validating HPA behavior.
- k6 Operator installed when using `LOADTEST_K6_MODE=operator`; the default
  smoke path runs k6 as a plain Kubernetes Job.
- KEDA installed only for future queue/custom-metric scaling profiles.

Local kind or minikube is suitable for the `smoke` profile. The `100k`, `250k`,
and `500k` profiles are capacity targets for a real cluster with enough node
capacity.

## Quick Start

```bash
# Deploy dependencies, generated secrets, go-oauth2-server, and separate
# AuthProxy admin/api/public/worker releases into authproxy-load.
./loadtest/scripts/up smoke

# No-op for now except for artifact metadata; #718 fills in real state seeding.
./loadtest/scripts/seed smoke

# Run a small k6 smoke test against AuthProxy health endpoints and provider
# health. Uses a Kubernetes Job unless LOADTEST_K6_MODE=operator is set.
./loadtest/scripts/run smoke

# Capture pods, deployments, services, Helm values, logs, and k6 summary JSON.
./loadtest/scripts/collect smoke

# Remove Helm releases and load-test manifests. The namespace is kept by default.
./loadtest/scripts/down smoke
```

Set `LOADTEST_RUN_DIR=/path/to/run` to force all scripts to write into the same
artifact directory. Without it, each command creates a timestamped directory
under `loadtest/runs/`.

## Profiles

Profiles live in `profiles/`:

- `smoke.yaml` deploys the smallest environment and runs a low-rate k6 health
  test.
- `100k.yaml` targets 100,000 connections and 50,000-100,000 namespaces.
- `250k.yaml` targets 250,000 connections and 100,000+ namespaces.
- `500k.yaml` is the stretch profile.

The current scripts use the profile for run metadata and namespace selection.
Future seeding and k6 issues will consume the object-count and traffic sections
directly.

## Environment Variables

- `LOADTEST_NAMESPACE`: overrides the namespace from the profile.
- `LOADTEST_RUN_DIR`: writes artifacts to a fixed directory.
- `AUTHPROXY_IMAGE_REPOSITORY`: defaults to `ghcr.io/rmorlok/authproxy`.
- `AUTHPROXY_IMAGE_TAG`: defaults to `main`.
- `GO_OAUTH2_SERVER_IMAGE`: defaults to the current demo provider image.
- `K6_IMAGE`: defaults to `grafana/k6:0.54.0`.
- `LOADTEST_K6_MODE`: `job` (default) or `operator`.
- `LOADTEST_K6_TIMEOUT`: defaults to `5m`.
- `LOADTEST_INSTALL_K6_OPERATOR=true`: install or upgrade the k6 Operator with
  Helm during `up`.
- `LOADTEST_INSTALL_KEDA=true`: install or upgrade KEDA with Helm during `up`.

## What `up` Deploys

The smoke environment installs:

- Postgres for AuthProxy's main database.
- Redis for Asynq, sessions, rate-limit state, and task queues.
- ClickHouse as a placeholder app-metrics backend for future pressure tests.
- Grafana's `otel-lgtm` image for local OTel, Prometheus, Grafana, Tempo, and
  Loki endpoints.
- `go-oauth2-server` in test mode.
- Four separate AuthProxy Helm releases:
  - `authproxy-admin-api`
  - `authproxy-api`
  - `authproxy-public`
  - `authproxy-worker`

The chart's current model is one Deployment per release. Installing separate
releases lets later issues add independent autoscaling and per-service resource
tuning without changing this harness.

## Run Artifacts

Each script writes or appends to a run directory containing:

- `metadata.env`: command, timestamps, namespace, image references, and profile.
- `profile.yaml`: exact profile used for the run.
- `helm-values/`: the value overlays used by `up`.
- `kubernetes/`: resource snapshots, events, and rollout summaries.
- `helm/`: `helm list`, rendered values, and manifest snapshots.
- `k6/`: k6 logs and summary JSON when available.

These artifacts are the handoff point for follow-up optimization work such as DB
indexes, keyset pagination, request-event buffering, or worker tuning.

## Optional k6 Operator Mode

Install the k6 Operator in the cluster, or let `up` install it:

```bash
LOADTEST_INSTALL_K6_OPERATOR=true ./loadtest/scripts/up smoke
```

Then run:

```bash
LOADTEST_K6_MODE=operator ./loadtest/scripts/run smoke
```

The script creates the k6 script ConfigMap and applies
`k8s/k6/smoke-testrun.yaml`. The default Job mode is kept so smoke tests work on
clusters where the operator CRDs are not installed.

## Optional KEDA

KEDA is not needed for the smoke profile, but future worker queue and custom
metric scaling tests can install it during environment setup:

```bash
LOADTEST_INSTALL_KEDA=true ./loadtest/scripts/up smoke
```

## Cleanup

`down` uninstalls the four AuthProxy Helm releases and deletes the load-test
manifests. To remove the namespace as well:

```bash
./loadtest/scripts/down smoke --delete-namespace
```
