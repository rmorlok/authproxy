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

loadtest_init_run_dir() {
  local profile_name=$1
  local profile_file=$2
  local namespace=$3
  local command_name=$4
  local now
  now=$(loadtest_timestamp)

  local run_dir="${LOADTEST_RUN_DIR:-$LOADTEST_ROOT/runs/${now}-${profile_name}-${command_name}}"
  mkdir -p "$run_dir/helm-values" "$run_dir/kubernetes" "$run_dir/helm" "$run_dir/k6"
  cp "$profile_file" "$run_dir/profile.yaml"
  cp "$LOADTEST_ROOT"/helm-values/*.yaml "$run_dir/helm-values/"

  {
    printf "command=%s\n" "$command_name"
    printf "started_at=%s\n" "$now"
    printf "profile=%s\n" "$profile_name"
    printf "profile_file=%s\n" "$profile_file"
    printf "namespace=%s\n" "$namespace"
    printf "authproxy_image_repository=%s\n" "${AUTHPROXY_IMAGE_REPOSITORY:-ghcr.io/rmorlok/authproxy}"
    printf "authproxy_image_tag=%s\n" "${AUTHPROXY_IMAGE_TAG:-main}"
    printf "go_oauth2_server_image=%s\n" "${GO_OAUTH2_SERVER_IMAGE:-ghcr.io/rmorlok/go-oauth2-server@sha256:d1e5bdae007259b0c02255f26efce595f23eb2142e7da8caa3dac544dd445cb3}"
    printf "k6_image=%s\n" "${K6_IMAGE:-grafana/k6:0.54.0}"
    printf "install_k6_operator=%s\n" "${LOADTEST_INSTALL_K6_OPERATOR:-false}"
    printf "install_keda=%s\n" "${LOADTEST_INSTALL_KEDA:-false}"
  } > "$run_dir/metadata.env"

  printf "%s\n" "$run_dir"
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

  kubectl -n "$namespace" get pods -o wide > "$run_dir/kubernetes/pods.txt" 2>&1 || true
  kubectl -n "$namespace" get deployments -o wide > "$run_dir/kubernetes/deployments.txt" 2>&1 || true
  kubectl -n "$namespace" get services -o wide > "$run_dir/kubernetes/services.txt" 2>&1 || true
  kubectl -n "$namespace" get hpa -o yaml > "$run_dir/kubernetes/hpa.yaml" 2>&1 || true
  kubectl -n "$namespace" get events --sort-by=.lastTimestamp > "$run_dir/kubernetes/events.txt" 2>&1 || true
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
