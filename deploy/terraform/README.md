# Terraform: AuthProxy AWS infrastructure

Two-module layout for the EKS deployment pipeline:

```
deploy/terraform/
├── bootstrap/   # S3 state bucket + DynamoDB lock table (local state, one-time)
└── eks/        # VPC, EKS, IAM OIDC, Route53 (S3 remote state)
```

| Module        | Backend          | Apply order | Why                                            |
|---------------|------------------|-------------|------------------------------------------------|
| `bootstrap/`  | local            | First       | Creates the resources the eks/ backend uses    |
| `eks/`        | S3 (from bootstrap) | Second   | The main infrastructure                        |

See each module's README for details. This file covers the cross-module
apply procedure and one-time manual steps. For ongoing operations
(granting kubectl access, rotating credentials, recovering from stuck
deploys, cost monitoring), see the
[`EKS runbook`](../../docs/src/content/docs/deployment/eks-runbook.md).

## Steady-state cost

Targeting **~$150/mo** with the defaults:

| Item                             | Approx /mo |
|----------------------------------|------------|
| EKS control plane                | $73        |
| 2× t3.medium on-demand           | $60        |
| Single NAT gateway               | $32        |
| Route53 hosted zone              | $0.50      |
| S3 + DynamoDB (state)            | <$1        |
| **Total**                        | **~$165**  |

(Slightly over the project budget; flip `node_group_desired_size` to 1
to drop ~$30 if needed.)

## Apply (first time)

### 1. Bootstrap

```bash
cd deploy/terraform/bootstrap
terraform init
terraform apply
terraform output    # save state_bucket_name, lock_table_name
```

### 2. Wire the eks/ backend

Edit [`eks/backend.tf`](eks/backend.tf), replace
`REPLACE_ME_FROM_BOOTSTRAP_OUTPUT` with the `state_bucket_name` value
from above.

### 3. Apply eks/

```bash
cd ../eks
terraform init
terraform apply
terraform output    # save gh_actions_role_arn, route53_name_servers, kubeconfig_command
```

First apply takes ~15-20 min (EKS control plane + node group rollout).

### 4. Delegate the domain

The Route53 hosted zone for `authproxy.net` is created in step 3, but
the registrar is still authoritative until NS records are delegated.

```bash
terraform output route53_name_servers   # in deploy/terraform/eks
```

At the domain registrar (e.g. Namecheap, GoDaddy, etc.), replace the
existing NS records for `authproxy.net` with the four returned here.

### 5. Wire kubectl locally

```bash
$(terraform output -raw kubeconfig_command)
kubectl get nodes
```

### 6. Store the GH Actions role ARN as a repo secret

```bash
terraform output gh_actions_role_arn
gh secret set AWS_ROLE_ARN --body "<paste the arn>"
```

### 7. Verify the OIDC chain end-to-end

Push a manual run of [`.github/workflows/verify-eks-oidc.yml`](../../.github/workflows/verify-eks-oidc.yml):

```bash
gh workflow run "Verify EKS OIDC"
```

(Or use the Actions UI → "Verify EKS OIDC" → "Run workflow".) A green
run = the federated trust policy is wired correctly.

### 8. Install the in-cluster bootstrap layer

Once the cluster is up, install ingress-nginx + cert-manager + external-dns
+ metrics-server via the bootstrap chart. See
[`../charts/bootstrap/README.md`](../charts/bootstrap/README.md) for the
full procedure. Quick version:

```bash
cd ../charts/bootstrap
helm dependency update
helm upgrade --install authproxy-bootstrap . \
  --namespace kube-system \
  --wait --timeout 5m \
  --set "global.acmeEmail=you@example.com" \
  --set "global.hostedZoneId=$(cd ../../terraform/eks && terraform output -raw route53_zone_id)" \
  --set "global.domain=$(cd ../../terraform/eks && terraform output -raw domain_name)" \
  --set "external-dns.serviceAccount.annotations.eks\.amazonaws\.com/role-arn=$(cd ../../terraform/eks && terraform output -raw external_dns_role_arn)" \
  --set "external-dns.domainFilters[0]=$(cd ../../terraform/eks && terraform output -raw domain_name)" \
  --set "external-dns.zoneIdFilters[0]=$(cd ../../terraform/eks && terraform output -raw route53_zone_id)"
```

## Day-2: routine `terraform apply`

For routine config drift correction or version bumps, re-run `terraform
apply` in `eks/`. No `bootstrap/` changes are needed in normal operation.

## Destroying

```bash
cd deploy/terraform/eks
terraform destroy
```

The `eks/` module is fully reversible. The `bootstrap/` module has
`prevent_destroy = true` on the state bucket — see its README for the
careful tear-down procedure.

Before destroying:

- **Revert the Route53 NS delegation** at the registrar (point back to
  the registrar's default NS or to the next hosting provider). Otherwise
  the domain will resolve to a deleted zone.
- **Drain any workloads** (`helm uninstall` everything) so PVCs and
  load balancers are released cleanly. Stranded ELBs cost money even
  after `terraform destroy`.
