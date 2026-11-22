provider "aws" {
  # CloudFront requires its ACM certificate to be in us-east-1.
  region              = "us-east-1"
  allowed_account_ids = [local.account_id]

  default_tags {
    tags = {
      Project   = "authproxy"
      Component = "apex-redirect"
      ManagedBy = "terraform"
    }
  }
}

locals {
  account_id  = "092466876164"
  domain_name = "authproxy.net"
  # Pin the zone whose nameservers match the registry delegation. Route 53
  # allows multiple public zones with the same name.
  zone_id         = "Z08021412F8T4LFEINQL"
  deployment_name = "authproxy-apex-redirect"
  s3_origin_id    = "empty-private-s3"
}

variable "publish_dns" {
  description = "Publish the apex aliases. Disable for the initial deployment to test the redirect before routing public traffic."
  type        = bool
  default     = true
}

resource "aws_acm_certificate" "apex" {
  domain_name       = local.domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "certificate_validation" {
  for_each = {
    for option in aws_acm_certificate.apex.domain_validation_options : option.domain_name => option
  }

  zone_id = local.zone_id
  name    = each.value.resource_record_name
  type    = each.value.resource_record_type
  ttl     = 60
  records = [each.value.resource_record_value]
}

resource "aws_acm_certificate_validation" "apex" {
  certificate_arn         = aws_acm_certificate.apex.arn
  validation_record_fqdns = [for record in aws_route53_record.certificate_validation : record.fqdn]
}

# CloudFront requires an origin. The function answers at the edge, and this
# empty REST origin has no read grant so accidental origin requests fail closed.
resource "aws_s3_bucket" "origin" {
  bucket = "${local.deployment_name}-${local.account_id}"
}

resource "aws_s3_bucket_public_access_block" "origin" {
  bucket                  = aws_s3_bucket.origin.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

data "aws_cloudfront_cache_policy" "disabled" {
  name = "Managed-CachingDisabled"
}

resource "aws_cloudfront_function" "redirect" {
  name    = local.deployment_name
  runtime = "cloudfront-js-2.0"
  comment = "Temporarily redirect the AuthProxy apex to the documentation homepage"
  publish = true
  code    = file("${path.module}/redirect.js")
}

resource "aws_cloudfront_distribution" "redirect" {
  comment             = "Temporary redirect for ${local.domain_name}"
  aliases             = [local.domain_name]
  enabled             = true
  is_ipv6_enabled     = true
  wait_for_deployment = true

  origin {
    # The AWS provider supplies an empty S3 origin config when sending the
    # request and omits the empty block when reading it back into state.
    domain_name = aws_s3_bucket.origin.bucket_regional_domain_name
    origin_id   = local.s3_origin_id
  }

  default_cache_behavior {
    target_origin_id = local.s3_origin_id
    allowed_methods  = ["GET", "HEAD"]
    cached_methods   = ["GET", "HEAD"]
    cache_policy_id  = data.aws_cloudfront_cache_policy.disabled.id
    # Have the function handle both schemes with one hop to HTTPS on the docs host.
    viewer_protocol_policy = "allow-all"

    function_association {
      event_type   = "viewer-request"
      function_arn = aws_cloudfront_function.redirect.arn
    }
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    acm_certificate_arn      = aws_acm_certificate_validation.apex.certificate_arn
    ssl_support_method       = "sni-only"
    minimum_protocol_version = "TLSv1.2_2021"
  }
}

resource "aws_route53_record" "apex" {
  for_each = var.publish_dns ? toset(["A", "AAAA"]) : toset([])

  zone_id = local.zone_id
  name    = local.domain_name
  type    = each.key

  alias {
    name                   = aws_cloudfront_distribution.redirect.domain_name
    zone_id                = aws_cloudfront_distribution.redirect.hosted_zone_id
    evaluate_target_health = false
  }
}

output "cloudfront_domain_name" {
  description = "CloudFront hostname for testing the apex certificate and redirect before publishing DNS."
  value       = aws_cloudfront_distribution.redirect.domain_name
}
