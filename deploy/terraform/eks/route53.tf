resource "aws_route53_zone" "primary" {
  name = var.domain_name

  comment = "Public hosted zone for ${var.domain_name}. Delegate the registrar's NS records to the four NS entries on this zone after first apply."
}
