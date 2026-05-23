module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.13"

  name = "${var.cluster_name}-vpc"
  cidr = var.vpc_cidr

  azs             = local.azs
  private_subnets = local.private_subnet_cidrs
  public_subnets  = local.public_subnet_cidrs

  enable_nat_gateway = true
  # Single NAT gateway keeps the steady-state cost target under ~$150/mo.
  # Multi-AZ NAT adds ~$32/mo per extra NAT — flip to true for production
  # HA once cost ceiling allows.
  single_nat_gateway = true

  enable_dns_hostnames = true
  enable_dns_support   = true

  # Required tags for the AWS Load Balancer Controller to discover subnets.
  public_subnet_tags = {
    "kubernetes.io/role/elb"                    = "1"
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
  }

  private_subnet_tags = {
    "kubernetes.io/role/internal-elb"           = "1"
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
  }
}
