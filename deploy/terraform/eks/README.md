# Terraform: EKS cluster + supporting infra

Provisions the EKS cluster the deployment pipeline runs against.

| Resource                | Notes                                                         |
|-------------------------|---------------------------------------------------------------|
| VPC                     | 3 AZs, public + private subnets, single NAT (~$32/mo)         |
| EKS                     | 1.36, managed node group (t3.large x 3, one per AZ, scale 3-6) |
| Cluster addons          | coredns, kube-proxy, vpc-cni, ebs-csi-driver                  |
| IAM OIDC provider       | Federates GH Actions from `rmorlok/authproxy`                 |
| GH Actions role         | Tag pushes (`v*`, `chart-v*`) + dispatches with `gha-eks` env |
| Route53 hosted zone     | `authproxy.net` (NS delegation done manually at registrar)    |

Steady-state cost target: **~$290/mo** (single NAT ~$33 + 3× t3.large ~$182 + EKS control plane ~$73),
plus t3 unlimited-mode CPU surplus charges under sustained load.

The demo's stateful workloads use zonal EBS volumes, so the node group keeps at
least one node per AZ (`node_group_min_size >= az_count`). With fewer nodes than
AZs, a node replacement can leave a volume's AZ without a node and its pod
unschedulable. t3.medium (17 pods, 4 GiB) is too small for the split demo: both
nodes reached the VPC CNI pod limit.

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
