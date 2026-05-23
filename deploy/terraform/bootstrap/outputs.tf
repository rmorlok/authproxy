output "state_bucket_name" {
  description = "Name of the S3 bucket holding remote Terraform state. Pass to the eks/ module's backend config."
  value       = aws_s3_bucket.state.id
}

output "lock_table_name" {
  description = "DynamoDB table name used for state locking."
  value       = aws_dynamodb_table.lock.name
}

output "region" {
  description = "AWS region the state bucket + lock table live in. Echoes var.region."
  value       = var.region
}
