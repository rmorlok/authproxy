module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.24"

  cluster_name    = var.cluster_name
  cluster_version = var.kubernetes_version

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  # Public endpoint stays on so a developer laptop + GH Actions can reach
  # the API server. Restrict to specific CIDRs for tighter posture.
  cluster_endpoint_public_access = true

  # Modern access-entry model (no aws-auth ConfigMap shenanigans). The
  # caller running `terraform apply` (and the GH Actions OIDC role
  # below) get cluster-admin via aws_eks_access_entry resources.
  authentication_mode                      = "API"
  enable_cluster_creator_admin_permissions = true

  cluster_addons = {
    coredns            = { most_recent = true }
    kube-proxy         = { most_recent = true }
    vpc-cni            = { most_recent = true }
    aws-ebs-csi-driver = { most_recent = true }
  }

  eks_managed_node_groups = {
    default = {
      instance_types = var.node_instance_types
      capacity_type  = "ON_DEMAND"

      min_size     = var.node_group_min_size
      desired_size = var.node_group_desired_size
      max_size     = var.node_group_max_size

      labels = {
        role = "general"
      }
    }
  }
}

# Grant the GH Actions OIDC role cluster-admin so the verification
# workflow can `kubectl get ns` (and the future deploy workflows can
# `helm upgrade --install`).
resource "aws_eks_access_entry" "gh_actions" {
  cluster_name  = module.eks.cluster_name
  principal_arn = aws_iam_role.gh_actions.arn
  type          = "STANDARD"
}

resource "aws_eks_access_policy_association" "gh_actions_admin" {
  cluster_name  = module.eks.cluster_name
  principal_arn = aws_iam_role.gh_actions.arn
  policy_arn    = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"

  access_scope {
    type = "cluster"
  }
}
