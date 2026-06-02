output "cluster_name" {
  description = "EKS cluster name (pass to `aws eks update-kubeconfig --name <this>`)."
  value       = module.eks.cluster_name
}

output "cluster_endpoint" {
  description = "EKS API server endpoint."
  value       = module.eks.cluster_endpoint
}

output "cluster_certificate_authority_data" {
  description = "Base64-encoded CA cert for the cluster API server."
  value       = module.eks.cluster_certificate_authority_data
}

output "region" {
  description = "AWS region. Echoes var.region; convenient for downstream scripts."
  value       = var.region
}

output "vpc_id" {
  description = "VPC id the cluster lives in."
  value       = module.vpc.vpc_id
}

output "gh_actions_role_arn" {
  description = "ARN of the IAM role GitHub Actions assumes via OIDC. Wire into role-to-assume in verify-eks-oidc.yml."
  value       = aws_iam_role.gh_actions.arn
}

output "route53_zone_id" {
  description = "Route53 hosted zone id for the public domain."
  value       = aws_route53_zone.primary.zone_id
}

output "domain_name" {
  description = "Public domain the Route53 hosted zone is authoritative for. Echoes var.domain_name; the bootstrap chart's `global.domain` and external-dns `domainFilters` reference this."
  value       = var.domain_name
}

output "route53_name_servers" {
  description = "Authoritative NS records for the hosted zone. Delegate the registrar's NS records to these."
  value       = aws_route53_zone.primary.name_servers
}

output "kubeconfig_command" {
  description = "Convenience: copy-paste command to wire local kubectl to this cluster."
  value       = "aws eks update-kubeconfig --region ${var.region} --name ${module.eks.cluster_name}"
}

output "external_dns_role_arn" {
  description = "ARN of the IRSA role the external-dns ServiceAccount assumes. Pass as the bootstrap chart's `externalDns.iamRoleArn` value."
  value       = aws_iam_role.external_dns.arn
}

output "cert_manager_route53_role_arn" {
  description = "ARN of the IRSA role cert-manager assumes for Route53 DNS-01 challenges."
  value       = aws_iam_role.cert_manager_route53.arn
}

output "oidc_provider_arn" {
  description = "ARN of the cluster's OIDC provider. Future IRSA roles bind their trust policy to this."
  value       = module.eks.oidc_provider_arn
}
