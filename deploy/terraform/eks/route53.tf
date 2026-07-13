resource "aws_route53_zone" "primary" {
  name = var.domain_name

  comment = "Public hosted zone for ${var.domain_name}. Delegate the registrar's NS records to the four NS entries on this zone after first apply."
}

resource "aws_route53_record" "docs" {
  zone_id = aws_route53_zone.primary.zone_id
  name    = "docs.${var.domain_name}"
  type    = "CNAME"
  ttl     = 300
  records = [local.github_pages_domain]
}
