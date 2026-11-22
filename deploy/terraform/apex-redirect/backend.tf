terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  # The bootstrap stack owns the backend. This key keeps the redirect's
  # lifecycle independent of the EKS and blog stacks.
  backend "s3" {
    bucket         = "authproxy-tf-state-092466876164"
    key            = "apex-redirect/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "authproxy-tf-locks"
    encrypt        = true
  }
}
