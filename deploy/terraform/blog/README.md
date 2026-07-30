# AuthProxy Blog infrastructure

This module creates the private S3 bucket, CloudFront distribution, ACM
certificate, DNS records, and GitHub Actions deployment role for
`https://blog.authproxy.net`. Its state is separate from EKS at
`blog/terraform.tfstate`.

The reusable website module is pinned to the immutable commit behind
[`v2.0.0`](https://github.com/rmorlok/terraform-aws-s3-cloudfront-website/releases/tag/v2.0.0).
It uses CloudFront Origin Access Control, so S3 is never a public website
endpoint.

## First apply

```bash
terraform init
terraform plan
terraform apply
```

Then configure the `blog-production` GitHub Environment:

1. Restrict deployment branches to `main` and leave reviewer approvals disabled
   so a merge can publish automatically.
2. Add `AWS_BLOG_DEPLOY_ROLE_ARN` as an environment secret from
   `terraform output -raw blog_deploy_role_arn`.
3. Add these environment variables:

   ```bash
   BLOG_BUCKET=$(terraform output -raw blog_bucket_name)
   BLOG_CLOUDFRONT_DISTRIBUTION_ID=$(terraform output -raw blog_cloudfront_distribution_id)
   AWS_REGION=us-east-1
   ```

The workflow only receives OIDC credentials in the `blog-production`
environment. Its IAM policy is limited to objects in this bucket and
invalidations for this distribution.

## Validation

```bash
terraform fmt -check -recursive
terraform validate
terraform plan
```

After applying, verify that `https://blog.authproxy.net` serves the site,
direct S3 object URLs are denied, and an unknown page returns HTTP 404.
