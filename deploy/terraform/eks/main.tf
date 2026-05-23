provider "aws" {
  region = var.region

  default_tags {
    tags = var.tags
  }
}

data "aws_availability_zones" "available" {
  state = "available"

  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

data "aws_caller_identity" "current" {}

locals {
  azs = slice(data.aws_availability_zones.available.names, 0, var.az_count)

  # Subnets carved out of the VPC CIDR. /16 split into:
  #   - private: 10.0.0.0/19, 10.0.32.0/19, 10.0.64.0/19    (nodes)
  #   - public:  10.0.96.0/20, 10.0.112.0/20, 10.0.128.0/20 (LB, NAT)
  private_subnet_cidrs = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 3, i)]
  public_subnet_cidrs  = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 4, i + 6)]
}
