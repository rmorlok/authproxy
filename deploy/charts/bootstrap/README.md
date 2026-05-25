# authproxy-bootstrap Helm chart

Cluster bootstrap layer for an AuthProxy EKS install. Installs the
in-cluster components every downstream workload depends on:

| Component       | Role                                                        |
|-----------------|-------------------------------------------------------------|
| ingress-nginx   | HTTP/HTTPS entry point, fronted by an AWS NLB               |
| cert-manager    | Automatic TLS issuance (Let's Encrypt prod, HTTP-01)        |
| external-dns    | Reconciles Ingress hosts → Route53 A records                |
| metrics-server  | Resource metrics for HPA + `kubectl top`                    |
| ClusterIssuer   | Let's Encrypt prod issuer named `letsencrypt-prod`          |

This is a **helm-of-helms** chart: each component above is an upstream
chart pinned as a `dependencies` entry in `Chart.yaml`. The only
template owned by this chart directly is `templates/cluster_issuer.yaml`.

## Pre-requisites

1. EKS cluster up (provisioned via [`deploy/terraform/eks/`](../../terraform/eks)).
2. External-dns IRSA role created and its ARN known:

   ```bash
   cd deploy/terraform/eks
   terraform output external_dns_role_arn
   terraform output route53_zone_id
   terraform output domain_name
   ```

3. Local `kubectl` wired to the cluster:

   ```bash
   $(terraform output -raw kubeconfig_command)
   ```

4. Helm 3.16+ installed.

## Install

```bash
cd deploy/charts/bootstrap

helm dependency update     # fetch the four upstream charts into charts/

helm upgrade --install authproxy-bootstrap . \
  --namespace kube-system \
  --create-namespace \
  --set "global.acmeEmail=you@example.com" \
  --set "global.hostedZoneId=$(cd ../../terraform/eks && terraform output -raw route53_zone_id)" \
  --set "global.domain=$(cd ../../terraform/eks && terraform output -raw domain_name)" \
  --set "external-dns.serviceAccount.annotations.eks\.amazonaws\.com/role-arn=$(cd ../../terraform/eks && terraform output -raw external_dns_role_arn)" \
  --set "external-dns.domainFilters[0]=$(cd ../../terraform/eks && terraform output -raw domain_name)"
```

The post-install NOTES print a verification checklist; follow them in
order.

## What "Ready" looks like

```bash
kubectl get pods -n kube-system \
  -l 'app.kubernetes.io/name in (ingress-nginx,cert-manager,external-dns,metrics-server)'
```

All four `app.kubernetes.io/name` selectors should report Running pods.

```bash
kubectl get clusterissuer letsencrypt-prod -o wide
```

`READY=True` once cert-manager has registered with Let's Encrypt (~10-30s
after the post-install hook fires).

## Smoke test — DNS + TLS round-trip

```bash
# 1. Substitute the real domain into the example:
sed "s/YOUR_DOMAIN/$(cd ../../terraform/eks && terraform output -raw domain_name)/g" \
  examples/hello-echo.yaml | kubectl apply -f -

# 2. Watch external-dns + cert-manager do their thing:
kubectl -n hello-echo get ingress,certificate -w
# (Ctrl-C once certificate READY=True; ~3 min on a healthy install.)

# 3. Verify end to end:
curl -fsS https://hello.$(cd ../../terraform/eks && terraform output -raw domain_name) | head
```

Tear down:

```bash
kubectl delete ns hello-echo
```

## Upgrades

```bash
helm dependency update   # if Chart.yaml's dependency versions changed
helm upgrade authproxy-bootstrap . --namespace kube-system [...same --set flags...]
```

Subchart version bumps are deliberate — change one `dependencies[*].version`
at a time and verify the smoke test still passes before merging.

## Uninstall

```bash
# Drain workloads first (cert-manager Certificates, Ingresses, etc.).
helm uninstall authproxy-bootstrap --namespace kube-system

# CRDs are retained by design (crds.keep=true on cert-manager); delete
# manually if you really want them gone:
kubectl get crd | grep cert-manager.io | awk '{print $1}' | xargs kubectl delete crd
```

## Why not Terraform `helm_release`?

We deliberately keep K8s state out of Terraform state:

- Helm's drift detection and `--dry-run` are richer than `terraform plan`
  for in-cluster objects.
- `terraform destroy` of a `helm_release` against a stopped cluster
  hangs forever.
- The bootstrap layer evolves on a different cadence than AWS infra;
  separate state files make per-layer upgrades cleaner.

Terraform stays the source of truth for AWS resources (VPC, EKS, IAM);
Helm owns everything inside the cluster.
