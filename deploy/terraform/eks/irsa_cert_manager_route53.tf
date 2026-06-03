# IRSA role for cert-manager DNS-01 challenges.
#
# cert-manager uses this role when a ClusterIssuer with the Route53 DNS-01
# solver issues wildcard certificates. It runs as the cert-manager controller
# ServiceAccount in kube-system and exchanges its EKS-issued JWT for STS
# credentials via IRSA.
#
# Scoped narrowly: change and list records only in the authproxy.net hosted
# zone, with Route53's required global read calls left at "*".

locals {
  cert_manager_namespace            = "kube-system"
  cert_manager_service_account_name = "authproxy-bootstrap-cert-manager"
}

data "aws_iam_policy_document" "cert_manager_route53_trust" {
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
      values   = ["system:serviceaccount:${local.cert_manager_namespace}:${local.cert_manager_service_account_name}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${module.eks.oidc_provider}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "cert_manager_route53_inline" {
  # DNS-01 needs to create and remove the _acme-challenge TXT records.
  statement {
    effect = "Allow"
    actions = [
      "route53:ChangeResourceRecordSets",
      "route53:ListResourceRecordSets",
    ]
    resources = ["arn:aws:route53:::hostedzone/${aws_route53_zone.primary.zone_id}"]
  }

  # These APIs do not support resource-level constraints.
  statement {
    effect = "Allow"
    actions = [
      "route53:GetChange",
      "route53:ListHostedZonesByName",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role" "cert_manager_route53" {
  name               = "${var.cluster_name}-cert-manager-route53"
  description        = "IRSA role assumed by cert-manager to solve Route53 DNS-01 challenges for ${var.domain_name}"
  assume_role_policy = data.aws_iam_policy_document.cert_manager_route53_trust.json
}

resource "aws_iam_role_policy" "cert_manager_route53" {
  name   = "${var.cluster_name}-cert-manager-route53"
  role   = aws_iam_role.cert_manager_route53.id
  policy = data.aws_iam_policy_document.cert_manager_route53_inline.json
}
