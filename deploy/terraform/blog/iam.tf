# The EKS Terraform state already creates the account's GitHub OIDC provider.
# Use it as data rather than creating a second provider for the same issuer.
data "aws_iam_openid_connect_provider" "github" {
  url = "https://token.actions.githubusercontent.com"
}

data "aws_iam_policy_document" "github_blog_deploy_trust" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    # GitHub Environment branch protection must limit blog-production to main.
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_repository}:environment:blog-production"]
    }
  }
}

resource "aws_iam_role" "github_blog_deploy" {
  name               = "authproxy-blog-gh-actions"
  description        = "GitHub Actions OIDC role for publishing the AuthProxy Blog."
  assume_role_policy = data.aws_iam_policy_document.github_blog_deploy_trust.json
  tags               = var.tags
}

data "aws_iam_policy_document" "github_blog_deploy" {
  statement {
    sid       = "ListBlogContent"
    effect    = "Allow"
    actions   = ["s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketMultipartUploads"]
    resources = [module.website.bucket_arn]
  }

  statement {
    sid    = "ManageBlogContent"
    effect = "Allow"
    actions = [
      "s3:AbortMultipartUpload",
      "s3:DeleteObject",
      "s3:GetObject",
      "s3:ListMultipartUploadParts",
      "s3:PutObject",
    ]
    resources = ["${module.website.bucket_arn}/*"]
  }

  statement {
    sid       = "InvalidateBlogDistribution"
    effect    = "Allow"
    actions   = ["cloudfront:CreateInvalidation"]
    resources = [module.website.cloudfront_distribution_arn]
  }
}

resource "aws_iam_role_policy" "github_blog_deploy" {
  name   = "authproxy-blog-deploy"
  role   = aws_iam_role.github_blog_deploy.id
  policy = data.aws_iam_policy_document.github_blog_deploy.json
}
