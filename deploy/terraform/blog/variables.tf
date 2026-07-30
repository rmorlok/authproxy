variable "region" {
  description = "CloudFront certificates and this website's resources are created in us-east-1."
  type        = string
  default     = "us-east-1"
}

variable "domain_name" {
  description = "Parent Route53 domain that already has an authoritative public hosted zone."
  type        = string
  default     = "authproxy.net"
}

variable "github_repository" {
  description = "GitHub repository allowed to deploy the blog through OIDC."
  type        = string
  default     = "rmorlok/authproxy"
}

variable "tags" {
  description = "Tags applied to the blog infrastructure."
  type        = map(string)
  default = {
    Project   = "authproxy"
    Component = "blog"
    ManagedBy = "terraform"
  }
}
