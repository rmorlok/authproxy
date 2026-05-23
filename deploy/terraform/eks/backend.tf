# Remote state — created by the ../bootstrap/ module. The exact bucket
# name is account-specific (suffixed with the account id); after
# `terraform apply` in bootstrap/, copy `state_bucket_name` from its
# output here, then run `terraform init -migrate-state` in this dir.
terraform {
  backend "s3" {
    bucket         = "REPLACE_ME_FROM_BOOTSTRAP_OUTPUT"
    key            = "eks/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "authproxy-tf-locks"
    encrypt        = true
  }
}
