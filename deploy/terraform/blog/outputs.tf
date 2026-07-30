output "blog_url" {
  description = "Public blog URL after certificate validation and distribution deployment complete."
  value       = module.website.website_url
}

output "blog_bucket_name" {
  description = "Set this as the BLOG_BUCKET GitHub Environment variable."
  value       = module.website.bucket_name
}

output "blog_cloudfront_distribution_id" {
  description = "Set this as the BLOG_CLOUDFRONT_DISTRIBUTION_ID GitHub Environment variable."
  value       = module.website.cloudfront_distribution_id
}

output "blog_deploy_role_arn" {
  description = "Set this as the AWS_BLOG_DEPLOY_ROLE_ARN GitHub Environment secret."
  value       = aws_iam_role.github_blog_deploy.arn
}
