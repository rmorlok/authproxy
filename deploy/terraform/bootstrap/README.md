# Terraform: Bootstrap

One-time module that creates the S3 bucket + DynamoDB lock table the
[`../eks/`](../eks) module uses as its remote backend.

## Why a separate module?

The chicken-and-egg of remote Terraform state: you need state-tracking
resources to track your other state. This module:

- Lives on **local** state (no `backend` block).
- Is run **once**, by hand, before any other module.
- Is intentionally `prevent_destroy = true` on the state bucket so
  `terraform destroy` here can't orphan downstream state.

## Apply

```bash
cd deploy/terraform/bootstrap
terraform init
terraform plan
terraform apply
```

Outputs the bucket name and lock table name — wire those into
`../eks/backend.tf`'s `terraform { backend "s3" { ... } }` block before
running `terraform init` in the eks/ module.

## Destroying

Don't, unless you're tearing down everything. The state bucket has
`prevent_destroy` set:

1. `terraform destroy` the eks/ module first.
2. Empty the state bucket (`aws s3 rm s3://<bucket> --recursive`).
3. Comment out `prevent_destroy` (or delete the bucket manually).
4. `terraform destroy` here.
