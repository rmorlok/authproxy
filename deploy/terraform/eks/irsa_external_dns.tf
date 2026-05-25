# IRSA role for external-dns (cluster bootstrap, A8 / #293).
#
# external-dns reads Ingress hosts in the cluster and writes corresponding
# Route53 records. It runs as a ServiceAccount in kube-system and
# authenticates to AWS via IRSA — the EKS-issued JWT for the SA gets
# exchanged for STS credentials with this role's permissions.
#
# Scoped narrowly: list all hosted zones (external-dns needs the global
# list call to find its target zone) + change records only on the
# authproxy.net zone.

locals {
  external_dns_namespace            = "kube-system"
  external_dns_service_account_name = "external-dns"
}

data "aws_iam_policy_document" "external_dns_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    effect  = "Allow"

    principals {
      type        = "Federated"
      identifiers = [module.eks.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${module.eks.oidc_provider}:sub"
      values   = ["system:serviceaccount:${local.external_dns_namespace}:${local.external_dns_service_account_name}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${module.eks.oidc_provider}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "external_dns_inline" {
  # Scoped to the single hosted zone — external-dns has no business
  # touching any other zone in the account.
  statement {
    effect    = "Allow"
    actions   = ["route53:ChangeResourceRecordSets"]
    resources = ["arn:aws:route53:::hostedzone/${aws_route53_zone.primary.zone_id}"]
  }

  # Read-only zone discovery + record listing. These APIs don't accept a
  # resource-level constraint, so "*" is the only valid form.
  statement {
    effect = "Allow"
    actions = [
      "route53:ListHostedZones",
      "route53:ListResourceRecordSets",
      "route53:ListTagsForResource",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role" "external_dns" {
  name               = "${var.cluster_name}-external-dns"
  description        = "IRSA role assumed by the external-dns SA in kube-system to manage Route53 records for ${var.domain_name}"
  assume_role_policy = data.aws_iam_policy_document.external_dns_trust.json
}

resource "aws_iam_role_policy" "external_dns" {
  name   = "${var.cluster_name}-external-dns-route53"
  role   = aws_iam_role.external_dns.id
  policy = data.aws_iam_policy_document.external_dns_inline.json
}
