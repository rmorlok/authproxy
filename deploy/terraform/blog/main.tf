provider "aws" {
  region = var.region

  default_tags {
    tags = var.tags
  }
}

data "aws_caller_identity" "current" {}

data "aws_route53_zone" "primary" {
  name         = "${var.domain_name}."
  private_zone = false
}

locals {
  blog_domain = "blog.${var.domain_name}"
  bucket_name = "authproxy-blog-${data.aws_caller_identity.current.account_id}"
}

module "website" {
  # v2.0.0: private S3 origin with CloudFront OAC, DNS validation, and real 404s.
  # Pinning the release commit avoids moving-branch module resolution in Terraform.
  source = "git::https://github.com/rmorlok/terraform-aws-s3-cloudfront-website.git?ref=01d5a61a974a00af48851f9fa393dcb5e55dbe61"

  deployment_name = "authproxy-blog"
  s3_bucket_name  = local.bucket_name
  domain_names    = [local.blog_domain]
  route53_zone_id = data.aws_route53_zone.primary.zone_id
  tags            = var.tags
}
