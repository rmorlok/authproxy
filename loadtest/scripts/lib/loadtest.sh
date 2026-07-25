#!/usr/bin/env bash
set -euo pipefail

LOADTEST_SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
LOADTEST_ROOT=$(cd "$LOADTEST_SCRIPT_DIR/../.." && pwd)
REPO_ROOT=$(cd "$LOADTEST_ROOT/.." && pwd)

LOADTEST_DEFAULT_NAMESPACE=authproxy-load
LOADTEST_AUTH_PROXY_RELEASES=(
  authproxy-admin-api
  authproxy-api
  authproxy-public
  authproxy-worker
)

loadtest_log() {
  printf "[loadtest] %s\n" "$*"
}

loadtest_die() {
  printf "[loadtest] ERROR: %s\n" "$*" >&2
  exit 1
}

loadtest_require_cmd() {
  local cmd=$1
  command -v "$cmd" >/dev/null 2>&1 || loadtest_die "required command not found: $cmd"
}

loadtest_run_cli() {
  if [[ -n "${LOADTEST_CLI_BIN:-}" ]]; then
    "$LOADTEST_CLI_BIN" "$@"
    return
  fi

  (
    cd "$REPO_ROOT"
    go run ./cmd/loadtest "$@"
  )
}

loadtest_profile_path() {
  local profile=$1
  if [[ -f "$profile" ]]; then
    printf "%s\n" "$profile"
    return
  fi

  local candidate="$LOADTEST_ROOT/profiles/${profile}.yaml"
  [[ -f "$candidate" ]] || loadtest_die "unknown profile: $profile"
  printf "%s\n" "$candidate"
}

loadtest_yaml_top_value() {
  local file=$1
  local key=$2
  awk -v key="$key" '
    $0 ~ "^[[:space:]]*" key ":" {
      sub("^[^:]*:[[:space:]]*", "")
      gsub(/^["'\'']|["'\'']$/, "")
      print
      exit
    }
  ' "$file"
}

loadtest_yaml_section_value() {
  local file=$1
  local section=$2
  local key=$3
  awk -v section="$section" -v key="$key" '
    function indent(line, copy) {
      copy = line
      sub(/[^ ].*$/, "", copy)
      return length(copy)
    }
    function clean_value(value) {
      sub(/^[^:]*:[[:space:]]*/, "", value)
      sub(/[[:space:]]+#.*$/, "", value)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      gsub(/^["\047]|["\047]$/, "", value)
      return value
    }
    /^[[:space:]]*($|#)/ { next }
    indent($0) == 0 && $0 ~ "^" section ":[[:space:]]*($|#)" {
      in_section = 1
      section_indent = indent($0)
      next
    }
    in_section {
      current_indent = indent($0)
      if (current_indent <= section_indent) {
        exit
      }
      if (current_indent == section_indent + 2 && $0 ~ "^[[:space:]]*" key ":") {
        print clean_value($0)
        exit
      }
    }
  ' "$file"
}

loadtest_yaml_section_list() {
  local file=$1
  local section=$2
  local key=$3
  awk -v section="$section" -v key="$key" '
    function indent(line, copy) {
      copy = line
      sub(/[^ ].*$/, "", copy)
      return length(copy)
    }
    function trim(value) {
      sub(/[[:space:]]+#.*$/, "", value)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      gsub(/^["\047]|["\047]$/, "", value)
      return value
    }
    /^[[:space:]]*($|#)/ { next }
    indent($0) == 0 && $0 ~ "^" section ":[[:space:]]*($|#)" {
      in_section = 1
      section_indent = indent($0)
      next
    }
    in_section {
      current_indent = indent($0)
      if (current_indent <= section_indent) {
        exit
      }
      if (in_list) {
        if (current_indent <= key_indent) {
          exit
        }
        if ($0 ~ "^[[:space:]]*-[[:space:]]*") {
          value = $0
          sub(/^[[:space:]]*-[[:space:]]*/, "", value)
          print trim(value)
        }
        next
      }
      if (current_indent == section_indent + 2 && $0 ~ "^[[:space:]]*" key ":") {
        value = $0
        sub(/^[^:]*:[[:space:]]*/, "", value)
        value = trim(value)
        if (value ~ /^\[/) {
          gsub(/^\[|\]$/, "", value)
          n = split(value, parts, ",")
          for (idx = 1; idx <= n; idx++) {
            print trim(parts[idx])
          }
          exit
        }
        if (value != "") {
          print value
          exit
        }
        in_list = 1
        key_indent = current_indent
      }
    }
  ' "$file"
}

loadtest_profile_name() {
  local profile_file=$1
  local name
  name=$(loadtest_yaml_top_value "$profile_file" name)
  printf "%s\n" "${name:-$(basename "$profile_file" .yaml)}"
}

loadtest_namespace() {
  local profile_file=$1
  local namespace
  namespace=$(loadtest_yaml_top_value "$profile_file" namespace)
  printf "%s\n" "${LOADTEST_NAMESPACE:-${namespace:-$LOADTEST_DEFAULT_NAMESPACE}}"
}

loadtest_timestamp() {
  date -u "+%Y%m%dT%H%M%SZ"
}

loadtest_sanitize_k8s_name() {
  printf "%s\n" "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9-]+/-/g; s/^-+//; s/-+$//; s/-+/-/g' | cut -c 1-50 | sed -E 's/-+$//'
}

loadtest_latest_seed_dataset() {
  local profile_name=$1
  local latest=""
  local candidate

  shopt -s nullglob
  for candidate in "$LOADTEST_ROOT"/runs/*-"$profile_name"-seed/datasets/connections.csv; do
    latest=$candidate
  done
  shopt -u nullglob

  if [[ -n "$latest" ]]; then
    printf "%s\n" "$latest"
  fi
}

loadtest_base64_decode() {
  if base64 --help 2>&1 | grep -q -- "--decode"; then
    base64 --decode
  else
    base64 -D
  fi
}

loadtest_init_run_dir() {
  local profile_name=$1
  local profile_file=$2
  local namespace=$3
  local command_name=$4
  local now
  now=$(loadtest_timestamp)

  local run_dir="${LOADTEST_RUN_DIR:-$LOADTEST_ROOT/runs/${now}-${profile_name}-${command_name}}"
  mkdir -p \
    "$run_dir/helm-values" \
    "$run_dir/kubernetes" \
    "$run_dir/helm" \
    "$run_dir/k6" \
    "$run_dir/observability/dashboards" \
    "$run_dir/prometheus/queries" \
    "$run_dir/db-explain/sql"
  cp "$profile_file" "$run_dir/profile.yaml"
  cp "$LOADTEST_ROOT"/helm-values/*.yaml "$run_dir/helm-values/"
  cp "$LOADTEST_ROOT/grafana/dashboards.yaml" "$run_dir/observability/"
  cp "$LOADTEST_ROOT/grafana/datasources.yaml" "$run_dir/observability/"
  cp "$LOADTEST_ROOT/grafana/dashboards"/*.json "$run_dir/observability/dashboards/"
  cp "$LOADTEST_ROOT/prometheus/alerts.yaml" "$run_dir/prometheus/"
  cp "$LOADTEST_ROOT/prometheus/queries.tsv" "$run_dir/prometheus/"
  cp "$LOADTEST_ROOT/sql"/*.sql "$run_dir/db-explain/sql/"

  {
    printf "command=%s\n" "$command_name"
    printf "started_at=%s\n" "$now"
    printf "profile=%s\n" "$profile_name"
    printf "profile_file=%s\n" "$profile_file"
    printf "namespace=%s\n" "$namespace"
    printf "authproxy_image_repository=%s\n" "${AUTHPROXY_IMAGE_REPOSITORY:-ghcr.io/rmorlok/authproxy}"
    printf "authproxy_image_tag=%s\n" "${AUTHPROXY_IMAGE_TAG:-main}"
    printf "go_oauth2_server_image=%s\n" "${GO_OAUTH2_SERVER_IMAGE:-ghcr.io/rmorlok/go-oauth2-server:master}"
    printf "k6_image=%s\n" "${K6_IMAGE:-grafana/k6:0.54.0}"
    printf "prometheus_url=%s\n" "${LOADTEST_PROMETHEUS_URL:-service/prometheus-loadtest}"
    printf "install_k6_operator=%s\n" "${LOADTEST_INSTALL_K6_OPERATOR:-false}"
    printf "install_keda=%s\n" "${LOADTEST_INSTALL_KEDA:-false}"
  } > "$run_dir/metadata.env"

  printf "%s\n" "$run_dir"
}

loadtest_profile_p95_seconds() {
  local profile_file=$1
  local value
  value=$(loadtest_yaml_section_value "$profile_file" k6 p95_latency_ms)
  if [[ -z "$value" ]]; then
    value=$(loadtest_yaml_section_value "$profile_file" acceptance p95_latency_target)
  fi

  case "$value" in
    ""|profile-configured)
      printf "1\n"
      ;;
    *ms)
      awk -v milliseconds="${value%ms}" 'BEGIN { printf "%.6f\n", milliseconds / 1000 }'
      ;;
    *s)
      printf "%s\n" "${value%s}"
      ;;
    *)
      awk -v milliseconds="$value" 'BEGIN { printf "%.6f\n", milliseconds / 1000 }'
      ;;
  esac
}

loadtest_apply_observability_assets() {
  local namespace=$1
  local profile_file=$2
  local run_dir=$3
  local tmp
  local proxy_p95_seconds
  tmp=$(mktemp -d)
  proxy_p95_seconds=$(loadtest_profile_p95_seconds "$profile_file")

  kubectl -n "$namespace" create configmap authproxy-loadtest-grafana-provisioning \
    --from-file=datasources.yaml="$LOADTEST_ROOT/grafana/datasources.yaml" \
    --from-file=dashboards.yaml="$LOADTEST_ROOT/grafana/dashboards.yaml" \
    --dry-run=client -o yaml > "$tmp/grafana-provisioning.yaml"
  kubectl -n "$namespace" apply -f "$tmp/grafana-provisioning.yaml"

  kubectl -n "$namespace" create configmap authproxy-loadtest-grafana-dashboards \
    --from-file="$LOADTEST_ROOT/grafana/dashboards" \
    --dry-run=client -o yaml > "$tmp/grafana-dashboards.yaml"
  kubectl -n "$namespace" apply -f "$tmp/grafana-dashboards.yaml"

  sed "s/> 1 # LOADTEST_PROXY_P95_SECONDS/> $proxy_p95_seconds/" \
    "$LOADTEST_ROOT/prometheus/alerts.yaml" > "$tmp/alerts.yaml"
  cp "$tmp/alerts.yaml" "$run_dir/prometheus/alerts.rendered.yaml"
  printf "proxy_p95_target_seconds=%s\n" "$proxy_p95_seconds" >> "$run_dir/metadata.env"

  kubectl -n "$namespace" create configmap authproxy-loadtest-prometheus-rules \
    --from-file=alerts.yaml="$tmp/alerts.yaml" \
    --dry-run=client -o yaml > "$tmp/prometheus-rules.yaml"
  kubectl -n "$namespace" apply -f "$tmp/prometheus-rules.yaml"

  rm -rf "$tmp"
}

loadtest_ensure_namespace() {
  local namespace=$1
  if kubectl get namespace "$namespace" >/dev/null 2>&1; then
    loadtest_log "namespace exists: $namespace"
  else
    loadtest_log "creating namespace: $namespace"
    kubectl create namespace "$namespace"
  fi
}

loadtest_apply_manifest_dir() {
  local namespace=$1
  local dir=$2
  local manifest

  for manifest in "$dir"/*.yaml; do
    loadtest_log "applying $(basename "$manifest")"
    kubectl -n "$namespace" apply -f "$manifest"
  done
}

loadtest_wait_for_deployment() {
  local namespace=$1
  local deployment=$2
  local timeout=${3:-5m}

  loadtest_log "waiting for deployment/$deployment"
  kubectl -n "$namespace" rollout status "deployment/$deployment" "--timeout=$timeout"
}

loadtest_ensure_generated_secrets() {
  local namespace=$1
  local tmp
  tmp=$(mktemp -d)

  local db_password=${LOADTEST_DB_PASSWORD:-authproxy-load}
  local redis_password=${LOADTEST_REDIS_PASSWORD:-authproxy-load}
  local clickhouse_password=${LOADTEST_CLICKHOUSE_PASSWORD:-authproxy-load}

  kubectl -n "$namespace" create secret generic authproxy-load-db \
    --from-literal=AUTHPROXY_DB_PASSWORD="$db_password" \
    --dry-run=client -o yaml > "$tmp/db.yaml"
  kubectl -n "$namespace" apply -f "$tmp/db.yaml"

  kubectl -n "$namespace" create secret generic authproxy-load-redis \
    --from-literal=AUTHPROXY_REDIS_PASSWORD="$redis_password" \
    --dry-run=client -o yaml > "$tmp/redis.yaml"
  kubectl -n "$namespace" apply -f "$tmp/redis.yaml"

  kubectl -n "$namespace" create secret generic authproxy-load-clickhouse \
    --from-literal=CLICKHOUSE_PASSWORD="$clickhouse_password" \
    --dry-run=client -o yaml > "$tmp/clickhouse.yaml"
  kubectl -n "$namespace" apply -f "$tmp/clickhouse.yaml"

  openssl genrsa -out "$tmp/system" 2048 >/dev/null 2>&1
  openssl rsa -in "$tmp/system" -pubout -out "$tmp/system.pub" >/dev/null 2>&1
  kubectl -n "$namespace" create secret generic authproxy-load-jwt \
    --from-file=system="$tmp/system" \
    --from-file=system.pub="$tmp/system.pub" \
    --dry-run=client -o yaml > "$tmp/jwt.yaml"
  kubectl -n "$namespace" apply -f "$tmp/jwt.yaml"

  openssl genrsa -out "$tmp/loadtest-admin" 2048 >/dev/null 2>&1
  openssl rsa -in "$tmp/loadtest-admin" -pubout -out "$tmp/loadtest-admin.pub" >/dev/null 2>&1
  kubectl -n "$namespace" create secret generic authproxy-load-actors \
    --from-file=loadtest-admin="$tmp/loadtest-admin" \
    --from-file=loadtest-admin.pub="$tmp/loadtest-admin.pub" \
    --dry-run=client -o yaml > "$tmp/actors.yaml"
  kubectl -n "$namespace" apply -f "$tmp/actors.yaml"

  openssl rand -out "$tmp/global_aes.key" 32
  kubectl -n "$namespace" create secret generic authproxy-load-encryption \
    --from-file=global_aes.key="$tmp/global_aes.key" \
    --dry-run=client -o yaml > "$tmp/encryption.yaml"
  kubectl -n "$namespace" apply -f "$tmp/encryption.yaml"

  rm -rf "$tmp"
}

loadtest_capture_cluster_snapshot() {
  local namespace=$1
  local run_dir=$2
  local captured_at
  captured_at=$(loadtest_timestamp)

  kubectl -n "$namespace" get pods -o wide > "$run_dir/kubernetes/pods.txt" 2>&1 || true
  kubectl -n "$namespace" get pods -o yaml > "$run_dir/kubernetes/pods.yaml" 2>&1 || true
  kubectl -n "$namespace" get deployments -o wide > "$run_dir/kubernetes/deployments.txt" 2>&1 || true
  kubectl -n "$namespace" get deployments -o yaml > "$run_dir/kubernetes/deployments.yaml" 2>&1 || true
  kubectl -n "$namespace" get services -o wide > "$run_dir/kubernetes/services.txt" 2>&1 || true
  kubectl -n "$namespace" get hpa -o yaml > "$run_dir/kubernetes/hpa.yaml" 2>&1 || true
  kubectl -n "$namespace" get events --sort-by=.lastTimestamp > "$run_dir/kubernetes/events.txt" 2>&1 || true
  kubectl -n "$namespace" get replicasets -o wide > "$run_dir/kubernetes/replicasets.txt" 2>&1 || true
  kubectl -n "$namespace" top pods > "$run_dir/kubernetes/pod-resource-usage.txt" 2>&1 || true

  if [[ ! -s "$run_dir/kubernetes/hpa-timeline.tsv" ]]; then
    printf "captured_at\tnamespace\thpa\tmin_replicas\tcurrent_replicas\tdesired_replicas\tmax_replicas\n" > "$run_dir/kubernetes/hpa-timeline.tsv"
  fi
  kubectl -n "$namespace" get hpa \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.minReplicas}{"\t"}{.status.currentReplicas}{"\t"}{.status.desiredReplicas}{"\t"}{.spec.maxReplicas}{"\n"}{end}' \
    2>/dev/null | awk -v captured_at="$captured_at" -v namespace="$namespace" 'BEGIN { OFS="\t" } NF { print captured_at, namespace, $0 }' \
    >> "$run_dir/kubernetes/hpa-timeline.tsv" || true

  if [[ ! -s "$run_dir/kubernetes/image-shas.tsv" ]]; then
    printf "captured_at\tnamespace\tpod\tcontainer\timage\timage_id\trestarts\n" > "$run_dir/kubernetes/image-shas.tsv"
  fi
  kubectl -n "$namespace" get pods \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .status.containerStatuses[*]}{.name}{"\t"}{.image}{"\t"}{.imageID}{"\t"}{.restartCount}{"\n"}{end}{end}' \
    2>/dev/null | awk -v captured_at="$captured_at" -v namespace="$namespace" 'BEGIN { OFS="\t" } NF { print captured_at, namespace, $0 }' \
    >> "$run_dir/kubernetes/image-shas.tsv" || true
}

loadtest_capture_helm_snapshot() {
  local namespace=$1
  local run_dir=$2
  local release

  helm -n "$namespace" list > "$run_dir/helm/list.txt" 2>&1 || true
  for release in "${LOADTEST_AUTH_PROXY_RELEASES[@]}"; do
    helm -n "$namespace" get values "$release" --all > "$run_dir/helm/${release}-values.yaml" 2>&1 || true
    helm -n "$namespace" get manifest "$release" > "$run_dir/helm/${release}-manifest.yaml" 2>&1 || true
  done
}

loadtest_capture_prometheus_snapshot() {
  local namespace=$1
  local run_dir=$2
  local snapshot_dir="$run_dir/prometheus"
  local url=${LOADTEST_PROMETHEUS_URL:-}
  local port_forward_pid=""
  local port=${LOADTEST_PROMETHEUS_PORT:-19090}
  local port_forward_log="$snapshot_dir/port-forward.log"

  mkdir -p "$snapshot_dir/queries"

  if [[ -z "$url" ]]; then
    kubectl -n "$namespace" port-forward service/prometheus-loadtest "$port:9090" > "$port_forward_log" 2>&1 &
    port_forward_pid=$!
    url="http://127.0.0.1:$port"

    local attempt
    for attempt in $(seq 1 20); do
      if curl --connect-timeout 1 --max-time 3 -fsS "$url/-/ready" >/dev/null 2>&1; then
        break
      fi
      sleep 1
    done
  fi

  if curl --connect-timeout 2 --max-time 10 -fsS "$url/-/ready" >/dev/null 2>&1; then
    printf "url=%s\ncaptured_at=%s\n" "$url" "$(loadtest_timestamp)" > "$snapshot_dir/metadata.env"
    curl --connect-timeout 2 --max-time 30 -fsS "$url/api/v1/status/buildinfo" > "$snapshot_dir/buildinfo.json" 2>&1 || true
    curl --connect-timeout 2 --max-time 30 -fsS "$url/api/v1/targets" > "$snapshot_dir/targets.json" 2>&1 || true
    curl --connect-timeout 2 --max-time 30 -fsS "$url/api/v1/rules" > "$snapshot_dir/rules.json" 2>&1 || true
    curl --connect-timeout 2 --max-time 30 -fsS "$url/api/v1/alerts" > "$snapshot_dir/alerts.json" 2>&1 || true
    curl --connect-timeout 2 --max-time 30 -fsS "$url/api/v1/label/__name__/values" > "$snapshot_dir/metric-names.json" 2>&1 || true

    local query_name
    local query
    while IFS=$'\t' read -r query_name query; do
      [[ -n "$query_name" ]] || continue
      [[ "$query_name" == \#* ]] && continue
      curl --connect-timeout 2 --max-time 30 -fsSG "$url/api/v1/query" \
        --data-urlencode "query=$query" > "$snapshot_dir/queries/${query_name}.json" 2>&1 || true
    done < "$LOADTEST_ROOT/prometheus/queries.tsv"
  else
    printf "Prometheus was not ready at %s\n" "$url" > "$snapshot_dir/error.txt"
  fi

  if [[ -n "$port_forward_pid" ]]; then
    kill "$port_forward_pid" >/dev/null 2>&1 || true
    wait "$port_forward_pid" 2>/dev/null || true
  fi
}

loadtest_capture_postgres_explain() {
  local namespace=$1
  local run_dir=$2
  local sql_file
  local name
  local output

  for sql_file in "$LOADTEST_ROOT"/sql/*.sql; do
    name=$(basename "$sql_file" .sql)
    output="$run_dir/db-explain/${name}.txt"
    kubectl -n "$namespace" exec -i deployment/postgresql -- \
      sh -c 'PGPASSWORD="$POSTGRESQL_PASSWORD" psql -U authproxy -d authproxy -v ON_ERROR_STOP=1' \
      < "$sql_file" > "$output" 2>&1 || true
  done
}
