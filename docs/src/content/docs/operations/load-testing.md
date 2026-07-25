---
title: Load testing
description: Run repeatable AuthProxy cardinality, background-job, proxy-throughput, soak, and autoscaling tests.
---

The load-test harness establishes capacity baselines before product
optimizations. It seeds tenant-like AuthProxy state, directs traffic through
the proxy to a synthetic OAuth provider, and preserves the measurements needed
to explain a bottleneck. It is not a production benchmark: record the cluster,
images, profile, and upstream behavior with every result.

The harness lives in the repository's
[`loadtest/` directory](https://github.com/rmorlok/authproxy/tree/main/loadtest).
It uses k6 for request traffic, the `authproxy-loadtest` command for bulk
state, Kubernetes for the environment, and Prometheus/Grafana/OTel for
measurement.

## Choose a profile

Use `smoke` to verify that the harness and cluster wiring work. Use the other
profiles only on a capacity-sized Kubernetes cluster.

| Profile | Intended cluster | Seeded state | Proxy traffic | Purpose |
| --- | --- | --- | --- | --- |
| `smoke` | kind, minikube, or a small cluster | 10 namespaces and connections | 2 requests/s for 30 seconds | Verify deployment, script, and basic telemetry wiring. |
| `100k` | Real cluster | 50,000-100,000 namespaces, 100,000 connections, 1,000 stale setup connections | 500 requests/s for 10 minutes; 4 distributed k6 runners | Initial cardinality and capacity gate. |
| `250k` | Real cluster | At least 100,000 namespaces, 250,000 connections, 2,500 stale setup connections | 1,000 requests/s for 10 minutes; 8 runners | Database and worker pressure beyond the initial gate. |
| `500k` | Real cluster | At least 200,000 namespaces, 500,000 connections, 5,000 stale setup connections | 1,500 requests/s for 10 minutes; 12 runners | Stretch exploration of the measured upper bound. |

Each high-cardinality profile also runs refresh sweeps at 10%, 50%, and 100%
of tokens in the refresh window, and periodic-probe scheduler walks at 1%,
10%, and 100% of connections. Their proxy soak is two hours. Their spike is
four phases: two-minute ramp up, five-minute hold, two-minute ramp down, and
five-minute recovery.

The profile's `authproxy` replica values are planning metadata, not Helm
inputs. The initial deployment and HPA limits are defined in
[`loadtest/helm-values/`](https://github.com/rmorlok/authproxy/tree/main/loadtest/helm-values).
The `proxy-scale` scenario intentionally fixes the API HPA at each
`k6.scale_replicas` value, so it is the reproducible way to compare the API
replica counts configured by a profile. The 100k profile uses 2, 4, 8, and 16.
Adjust the Helm overlays before `up` when a run needs a different starting API
or worker capacity, and record that change with the artifacts.

## Prepare a cluster

The high-cardinality profiles need a real cluster with sufficient node headroom
for AuthProxy, Postgres, Redis, ClickHouse, OTel/Prometheus/Grafana, the test
provider, and k6 runners. Node autoscaling is cluster-owned: pending Pods show
that cluster capacity, rather than the application, is the limiting factor.

Before a run, verify:

- `kubectl`, `helm`, `openssl`, and `curl` are available to the operator.
- Metrics Server is installed and returning Pod resource metrics; the API HPA
  relies on it.
- The k6 Operator and its `TestRun` CRD are installed for distributed runs.
  `up` can install it with `LOADTEST_INSTALL_K6_OPERATOR=true`.
- The cluster can schedule the largest requested k6 parallelism and the API
  HPA maximum without unschedulable Pods.
- KEDA is optional. Install it with `LOADTEST_INSTALL_KEDA=true` only when
  evaluating queue or other custom-metric worker scaling.
- The target AuthProxy database, Redis, app-metrics store, encryption key, and
  actor signing keys used by the seeder are the same ones used by the deployed
  AuthProxy releases. Keep this environment isolated from production data.

`up` creates the `authproxy-load` namespace by default and deploys separate
`admin-api`, `api`, `public`, and `worker` releases. The admin API release
performs the initial schema and connector migrations. `go-oauth2-server` runs
in test mode as a separate upstream sink, so an upstream limit can be
identified separately from proxy capacity.

## Run a smoke test

The smoke path is the contributor-ready check. It confirms Kubernetes
deployment, service reachability, a small local seed, collection, and teardown.
It does not exercise proxy traffic through the deployed database because its
default seed config is intentionally local SQLite/miniredis.

```bash
export LOADTEST_RUN_DIR="$PWD/loadtest/runs/smoke-$(date -u +%Y%m%dT%H%M%SZ)"

./loadtest/scripts/up smoke
./loadtest/scripts/seed smoke
./loadtest/scripts/run smoke
./loadtest/scripts/collect smoke
./loadtest/scripts/down smoke
```

Set `LOADTEST_K6_MODE=operator` to exercise the distributed k6 path once the
Operator is installed. By default, smoke uses a normal Kubernetes Job so it
also works on a small cluster without the Operator.

## Run the 100k baseline

For a real cardinality or proxy run, pass the seeder an AuthProxy config whose
Postgres, Redis, app-metrics, encryption, and actor-key settings match the
deployed environment. Without it, `seed` creates an isolated SQLite/miniredis
environment and the generated connection IDs cannot be used by deployed API
Pods. Because the seeder runs locally, its store endpoints must be reachable
from the operator host (for example through temporary port forwards), and its
file-backed keys must be available locally. Use a disposable load-test database
and provider instance.

The following sequence keeps all artifacts in one directory. Configure
`LOADTEST_AUTHPROXY_CONFIG` before any command that seeds or enqueues
background work.

```bash
export LOADTEST_RUN_DIR="$PWD/loadtest/runs/100k-$(date -u +%Y%m%dT%H%M%SZ)"
export LOADTEST_AUTHPROXY_CONFIG=/secure/path/loadtest-authproxy.yaml
export LOADTEST_K6_MODE=operator
export LOADTEST_K6_WAIT=true

./loadtest/scripts/up 100k
./loadtest/scripts/seed 100k

./loadtest/scripts/run 100k proxy-raw
./loadtest/scripts/run 100k proxy-wrapped
./loadtest/scripts/run 100k proxy-scale
./loadtest/scripts/background 100k all
./loadtest/scripts/run 100k proxy-soak
./loadtest/scripts/run 100k proxy-spike

./loadtest/scripts/collect 100k
./loadtest/scripts/down 100k
```

Run `proxy-raw` before `proxy-wrapped` so the wrapper's added behavior can be
compared with an otherwise equivalent provider request. `proxy-scale` waits for
each API replica count to finish before moving to the next one. The run uses a
compact sample of 10,000 connection IDs by default because a ConfigMap-backed
k6 script has a Kubernetes size limit. Use `LOADTEST_K6_CONNECTION_ROWS` to
reduce that sample, or use `all` only after confirming the generated ConfigMap
will fit.

The 100k seed and all background variants can take materially longer than the
ten-minute proxy scenario. Duration depends on database storage, network
latency, CPU limits, and worker concurrency, so treat the generated
`seed-summary.json`, `background-summary.json`, and queue samples as the run's
timing record rather than assuming a fixed completion time.

## Observe and decide

Grafana provisions four dashboards under **AuthProxy Load Test**. Use them
while the run is active, then use the captured Prometheus data and summaries
for the final decision.

| Gate | Primary evidence | Pass condition |
| --- | --- | --- |
| Seed cardinality | `seed-summary.json`, verified samples, seed logs | The requested 100k profile creates and verifies its namespaces, actors, connector state, connections, tokens, probes, and k6 dataset. |
| Proxy quality | k6 summary, `http_req_duration`, `http_req_failed`, `proxy_5xx_rate`, `proxy_upstream_5xx_rate`, dropped iterations | 5xx stays below 0.1%, p95 is within the profile target (1 second for high-cardinality profiles), and k6 achieves the intended arrival rate without dropped iterations. |
| Horizontal capacity | `k6/scale-results.tsv`, Proxy and Autoscaling dashboard, `kubernetes/hpa-timeline.tsv` | Throughput improves materially as API replicas increase until a measured provider, database, Redis, app-metrics, or node-capacity limit is reached. |
| OAuth refresh sweep | Background Jobs dashboard, refresh counters, `background/queue-samples.tsv`, task timings | The sweep discovers and enqueues expiring tokens within one cron interval, and the queue drains before the next interval at the configured worker scale. |
| Periodic scheduler | scheduler task configuration, worker/task telemetry, Pod memory and restart data | The scheduler walk remains bounded at 100k scheduled probes and does not OOM or create an unbounded backlog. |
| Upstream isolation | Upstream Provider dashboard, provider request/latency/5xx metrics | Provider saturation is visible and distinguished from AuthProxy errors before judging proxy capacity. |
| Datastore health | Datastores dashboard, SQL/Redis telemetry, `db-explain/` artifacts | Pool waits, locks, Redis latency, and insert pressure are compatible with the achieved rate; query plans explain any slow resource walk. |
| Soak and spike | k6 summaries, HPA timeline, Pod restarts, queue and latency trends | The two-hour soak has no growing error, latency, restart, or backlog trend. The spike scales and recovers without leaving a sustained backlog or an elevated replica count. |

Prometheus alerts are useful investigation signals, not replacements for the
k6 thresholds. In particular, a failed-rate threshold, a provider 5xx rate,
or dropped iterations means the requested rate was not achieved even when the
cluster's HPA appears healthy.

## Diagnose common failures

| Symptom | Likely cause | Next check |
| --- | --- | --- |
| `seed` succeeds but proxy requests return missing connection or authorization errors | The seeder used its default local config rather than stores shared with the deployed releases. | Set `LOADTEST_AUTHPROXY_CONFIG`, reseed, and verify the artifact dataset against the deployed API. |
| HPA does not scale | Metrics Server, CPU requests, or custom metric availability is missing. | Inspect `kubectl -n authproxy-load describe hpa authproxy-api` and the HPA timeline. |
| k6 Pods or API Pods stay Pending | Cluster/node capacity is exhausted or a resource request cannot be scheduled. | Inspect namespace events and record the pending Pods as a cluster-capacity limit. |
| k6 fails before meaningful traffic | The Operator/CRD is unavailable, a ConfigMap is too large, or the dataset is missing. | Verify `TestRun` installation, reduce `LOADTEST_K6_CONNECTION_ROWS`, and confirm `connections.csv`. |
| Queue never drains | Worker replicas, concurrency, Redis, provider latency, or a failing task is limiting the run. | Compare queue samples, task duration/failure metrics, worker HPA state, and provider latency. |
| Proxy latency rises without API saturation | The provider, Postgres, Redis, app-metrics writes, or node networking is limiting. | Compare all four dashboards and inspect the captured query plans before changing application code. |

## Preserve the baseline

Keep the run directory with the profile, image references, rendered Helm
values, HPA timeline, k6 summaries, Prometheus snapshots, logs, and database
`EXPLAIN (ANALYZE, BUFFERS)` results. That evidence is what makes the next run
comparable.

Do not add an index, replace an offset walk with keyset pagination, buffer
request-event writes, or change worker concurrency solely because a large
profile exists. First identify the saturated component and query plan from the
artifacts, then track that optimization as a separate change with a before and
after run. This keeps the suite a capacity baseline rather than a collection of
unattributed tuning changes.
