# Terraform: EKS cluster + supporting infra

Provisions the EKS cluster the deployment pipeline runs against.

| Resource                | Notes                                                         |
|-------------------------|---------------------------------------------------------------|
| VPC                     | 3 AZs, public + private subnets, single NAT (~$32/mo)         |
| EKS                     | 1.34, managed node group (t3.medium x 2, scale 1-4)           |
| Cluster addons          | coredns, kube-proxy, vpc-cni, ebs-csi-driver                  |
| IAM OIDC provider       | Federates GH Actions from `rmorlok/authproxy`                 |
| GH Actions role         | Tag pushes (`v*`, `chart-v*`) + dispatches with `gha-eks` env |
| Route53 hosted zone     | `authproxy.net` (NS delegation done manually at registrar)    |

Steady-state cost target: **~$150/mo** (single NAT + 2× t3.medium + EKS control plane).

## Pre-requisites

1. Apply [`../bootstrap/`](../bootstrap) first — it creates the S3 state
   bucket + DynamoDB lock table this module uses as its backend.
2. Copy `state_bucket_name` from the bootstrap output into
   [`backend.tf`](backend.tf), replacing the `REPLACE_ME_FROM_BOOTSTRAP_OUTPUT`
   placeholder.

## Apply

```bash
cd deploy/terraform/eks
terraform init
terraform plan
terraform apply
```

First apply takes ~15-20 min (most of it the EKS control plane + node
group rollout).

## Post-apply: one-time manual steps

### 1. Delegate `authproxy.net` to the new Route53 zone

```bash
terraform output route53_name_servers
```

At the domain registrar, replace the existing NS records for
`authproxy.net` with the four returned here. DNS propagation usually
within an hour.

### 2. Wire local kubectl

```bash
$(terraform output -raw kubeconfig_command)
kubectl get nodes
```

### 3. Save the GH Actions role ARN

```bash
terraform output gh_actions_role_arn
```

Set as `AWS_ROLE_ARN` repo secret (or paste into
`.github/workflows/verify-eks-oidc.yml`'s `role-to-assume`).

## Destroying

```bash
terraform destroy
```

The Route53 hosted zone is deleted with the rest of the module —
remember to revert the registrar's NS records first if you've delegated.
The state bucket in `../bootstrap/` is `prevent_destroy = true`; see
its README to fully tear down.
