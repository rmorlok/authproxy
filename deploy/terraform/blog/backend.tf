terraform {
  # The bootstrap module owns this bucket and lock table. This blog has an
  # independent state key so it can be planned and applied without touching EKS.
  backend "s3" {
    bucket         = "authproxy-tf-state-092466876164"
    key            = "blog/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "authproxy-tf-locks"
    encrypt        = true
  }
}
