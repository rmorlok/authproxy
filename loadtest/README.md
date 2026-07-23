# AuthProxy Load-Test Harness

This directory contains the Kubernetes harness for the AuthProxy load-test
project tracked by #711. It is intentionally split from product
optimizations: the harness gives us repeatable environment setup, smoke
traffic, state seeding, proxy-QPS scenarios, and artifact capture. The
background-job suite adds repeatable refresh, scheduler, resource snapshot, and
cleanup pressure runs against the same seeded profiles.

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

# Seed namespaces, actors, a synthetic OAuth2 connector, connections, OAuth2
# tokens, and k6-ready datasets. With no config override this uses an
# ephemeral SQLite/miniredis AuthProxy config under the run directory.
./loadtest/scripts/seed smoke

# Run a small k6 smoke test against AuthProxy health endpoints and provider
# health. Uses a Kubernetes Job unless LOADTEST_K6_MODE=operator is set.
./loadtest/scripts/run smoke

# Run proxy-QPS scenarios against seeded connections. The high-cardinality
# profiles default to k6 Operator mode for distributed runs.
./loadtest/scripts/run 100k proxy-raw
./loadtest/scripts/run 100k proxy-wrapped
./loadtest/scripts/run 100k proxy-scale
./loadtest/scripts/run 100k proxy-soak
./loadtest/scripts/run 100k proxy-spike

# Run background-job scenarios. Enqueue/drain scenarios need a config that
# points at the same Postgres/Redis/app-metrics stores as running workers.
LOADTEST_AUTHPROXY_CONFIG=/path/to/loadtest-authproxy.yaml ./loadtest/scripts/background 100k all

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

The seed script consumes the object-count section directly. Proxy-QPS scenarios
consume the generated `datasets/connections.csv`, compact it to the columns k6
needs, and reuse that same sampled dataset across raw, wrapped, spike, soak,
and scale runs.

## Background Scenarios

`./loadtest/scripts/background <profile> <scenario>` supports:

- `all`: refresh sweeps, scheduler sync, resource snapshot, and stale setup
  cleanup.
- `refresh-sweep`: seeds each `objects.oauth_tokens_expiring_percent` value
  and enqueues the OAuth refresh sweep task.
- `scheduler-sync`: seeds each `objects.periodic_probe_percent` value and
  measures the periodic scheduler's connection walk without needing a worker.
- `resource-snapshot`: enqueues the app-metrics resource snapshot task.
- `stale-setup-cleanup`: seeds `objects.stale_setup_connections`, waits for the
  load-test setup TTL, then enqueues stale setup cleanup.
- `probe-outcome-cleanup`: optional cleanup-task walk over probe-enabled
  connections.

Refresh, resource snapshot, stale setup cleanup, and probe outcome cleanup are
queue-drain tests: they require `LOADTEST_AUTHPROXY_CONFIG` or
`AUTHPROXY_CONFIG` to point at stores shared with a running worker. If no config
is set, the script can still run `scheduler-sync` with a generated local
SQLite config; set `LOADTEST_BACKGROUND_WAIT_DRAIN=false` only when you want to
record enqueue/queue state without waiting for worker processing.

## k6 Proxy Scenarios

`./loadtest/scripts/run <profile> <scenario>` supports:

- `smoke`: low-rate health checks for the deployed services.
- `proxy-raw`: constant-arrival-rate traffic against
  `/api/v1/connections/{id}/_proxy_raw`.
- `proxy-wrapped`: constant-arrival-rate comparison traffic against
  `/api/v1/connections/{id}/_proxy`.
- `proxy-scale`: sequentially fixes `authproxy-api` replicas to the profile's
  `k6.scale_replicas` entries and runs `LOADTEST_PROXY_SCALE_SCENARIO`
  (`proxy-raw` by default) at each size.
- `proxy-soak`: long constant-arrival-rate run using `k6.soak_duration`.
- `proxy-spike`: ramping-arrival-rate run using the profile's spike knobs.

The proxy script calls the go-oauth2-server load sink under
`/test/load/resource/proxy/{connection_id}` and expects the provider to accept
seeded bearer tokens with the `at_` prefix. Override the sink behavior with
`K6_UPSTREAM_STATUS`, `K6_UPSTREAM_BYTES`, `K6_UPSTREAM_DELAY_MS`,
`K6_UPSTREAM_JITTER_MS`, and `K6_UPSTREAM_BEARER_PREFIX`.

The runner mints a scoped `connections:proxy` token from the generated
`authproxy-load-actors` secret. Set `LOADTEST_AUTHPROXY_BEARER_TOKEN` or
`AUTHPROXY_BEARER_TOKEN` to provide a token yourself.

k6 thresholds fail runs when:

- `http_req_duration` p95 exceeds `K6_P95_THRESHOLD_MS`.
- `http_req_failed`, `proxy_5xx_rate`, or `proxy_upstream_5xx_rate` exceed the
  configured rate.
- k6 drops iterations or observes any unexpected upstream status.

ConfigMap-backed k6 Operator scripts have a Kubernetes size ceiling, so the
runner defaults to a compact sample of 10,000 seeded connections. Tune that with
`LOADTEST_K6_CONNECTION_ROWS` or `k6.connection_rows`; use `all` only when the
generated ConfigMap remains below `LOADTEST_K6_CONFIGMAP_MAX_BYTES`. Grafana's
k6 Operator docs recommend a PVC or local-file based runner image for larger
multi-file suites:
https://grafana.com/docs/k6/latest/set-up/set-up-distributed-k6/usage/executing-k6-scripts-with-testrun-crd/

## Environment Variables

- `LOADTEST_NAMESPACE`: overrides the namespace from the profile.
- `LOADTEST_RUN_DIR`: writes artifacts to a fixed directory.
- `AUTHPROXY_IMAGE_REPOSITORY`: defaults to `ghcr.io/rmorlok/authproxy`.
- `AUTHPROXY_IMAGE_TAG`: defaults to `main`.
- `GO_OAUTH2_SERVER_IMAGE`: defaults to `ghcr.io/rmorlok/go-oauth2-server:master`;
  pin this to a digest for reproducible capacity runs.
- `K6_IMAGE`: defaults to `grafana/k6:0.54.0`.
- `LOADTEST_K6_MODE`: `job` (default) or `operator`.
- `LOADTEST_K6_TIMEOUT`: defaults to `5m`.
- `LOADTEST_K6_WAIT=true`: wait for k6 Operator `TestRun` completion. `proxy-scale`
  enables this automatically so replica measurements run sequentially.
- `LOADTEST_K6_CONNECTIONS_CSV`: explicit seeded `connections.csv` path for proxy
  runs. Defaults to the latest seed artifacts for the profile.
- `LOADTEST_K6_CONNECTION_ROWS`: number of seeded rows to compact into the k6
  dataset, or `all`.
- `LOADTEST_PROXY_MODE`: override proxy mode for soak/spike runs (`raw` or
  `wrapped`).
- `LOADTEST_PROXY_SCALE_SCENARIO`: scenario to run for each replica count in
  `proxy-scale`; defaults to `proxy-raw`.
- `LOADTEST_AUTHPROXY_CONFIG`: AuthProxy config used by `seed`. When unset,
  `seed` generates a local SQLite/miniredis config in the run directory.
- `LOADTEST_PROVIDER_BASE_URL`: provider URL written into seeded connector
  definitions; defaults to `http://go-oauth2-server:8080`.
- `LOADTEST_BACKGROUND_SCENARIO`: default background scenario for
  `scripts/background`.
- `LOADTEST_BACKGROUND_QUEUE`: Asynq queue to enqueue to and inspect; defaults
  to `default`.
- `LOADTEST_BACKGROUND_DRAIN_TIMEOUT`: maximum time to wait for queue drain;
  defaults to `30m`.
- `LOADTEST_BACKGROUND_POLL_INTERVAL`: queue sampling interval; defaults to
  `5s`.
- `LOADTEST_BACKGROUND_WAIT_DRAIN=false`: record enqueue/queue state without
  waiting for workers.
- `LOADTEST_BACKGROUND_SEED=false`: skip the per-scenario seed step when the
  target DB is already prepared.
- `LOADTEST_STALE_SETUP_CONNECTIONS`: override the profile's stale setup count
  for `stale-setup-cleanup`.
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
  - `authproxy-api` with HPA enabled for CPU-based proxy scaling
  - `authproxy-public`
  - `authproxy-worker` with HPA enabled for CPU and queue-depth metric examples

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
- `k6/`: k6 logs, generated TestRun manifests, environment snapshots, summary
  JSON when available, and `scale-results.tsv` for Job-backed replica sweeps.

The `seed` step also writes:

- `datasets/connections.csv`: connection IDs and metadata for k6 scenarios.
- `datasets/stale_setup_connections.csv`: setup-state rows used by stale
  cleanup scenarios.
- `datasets/namespaces.csv`: generated tenant namespaces.
- `datasets/actors.csv`: generated tenant actors.
- `seed-summary.json`: machine-readable counts, selected percentages, and
  verified samples.
- `seed-plan.txt`: human-readable seed summary.

The `background` step writes one directory per variant, plus
`background-runs.tsv` at the top level. Each variant contains its seed artifacts
and:

- `background/background-summary.json`: timing, enqueue/drain deltas, expected
  expiring-token counts, scheduler task counts, memory snapshots, and artifact
  paths.
- `background/queue-samples.tsv`: sampled queue size, pending/active/retry
  counts, totals, latency, and Redis memory usage.
- `background/scheduler-task-configs.tsv`: scheduler task counts by task type
  and cronspec for `scheduler-sync`.

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
LOADTEST_K6_MODE=operator ./loadtest/scripts/run 100k proxy-raw
```

The script creates the k6 script ConfigMap and applies a `TestRun`. Proxy
scenarios set `parallelism` from the profile or `K6_PARALLELISM`, and `proxy-scale`
waits for each distributed run to finish before moving to the next API replica
count. The default Job mode is kept so smoke tests and smaller proxy checks work
on clusters where the operator CRDs are not installed.

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
