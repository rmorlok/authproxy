# --- GitHub Actions OIDC provider --------------------------------------
#
# Federates GH Actions runs from rmorlok/authproxy into this AWS account.
# Trust is intentionally narrow per A7's choice "Tag + workflow_dispatch
# only" — no PR or main-branch apply.

data "tls_certificate" "github" {
  url = "https://token.actions.githubusercontent.com"
}

resource "aws_iam_openid_connect_provider" "github" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.github.certificates[0].sha1_fingerprint]
}

# --- IAM role assumed by GH Actions ------------------------------------

data "aws_iam_policy_document" "gh_actions_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    effect  = "Allow"

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    # Restrict to release tag pushes + manual dispatches:
    #   - refs/tags/v*         (app release tags)
    #   - refs/tags/chart-v*   (chart release tags)
    #   - environment:gha-eks  (workflow_dispatch with this environment)
    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values = [
        "repo:${var.github_repository}:ref:refs/tags/v*",
        "repo:${var.github_repository}:ref:refs/tags/chart-v*",
        "repo:${var.github_repository}:environment:gha-eks",
      ]
    }
  }
}

resource "aws_iam_role" "gh_actions" {
  name               = "${var.cluster_name}-gh-actions"
  description        = "Role assumed by GitHub Actions (rmorlok/authproxy) for cluster ops"
  assume_role_policy = data.aws_iam_policy_document.gh_actions_trust.json
}

# Minimum permissions for the verification workflow + future deploys:
#   - eks:DescribeCluster (for `aws eks update-kubeconfig`)
#   - ec2:DescribeRegions / sts:GetCallerIdentity (general sanity)
# Cluster-level RBAC is granted via the access entry in cluster.tf.
data "aws_iam_policy_document" "gh_actions_inline" {
  statement {
    effect = "Allow"
    actions = [
      "eks:DescribeCluster",
      "eks:ListClusters",
      "ec2:DescribeRegions",
      "sts:GetCallerIdentity",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "gh_actions_inline" {
  name   = "${var.cluster_name}-gh-actions-base"
  role   = aws_iam_role.gh_actions.id
  policy = data.aws_iam_policy_document.gh_actions_inline.json
}
