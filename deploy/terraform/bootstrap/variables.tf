variable "region" {
  description = "AWS region where the state bucket + lock table live."
  type        = string
  default     = "us-east-1"
}

variable "state_bucket_name" {
  description = <<-EOT
    Name of the S3 bucket that holds remote Terraform state for the eks/
    module. Must be globally unique. Default suffixes the caller's account
    id so the same name doesn't clash if this is ever forked.
  EOT
  type        = string
  default     = ""
}

variable "lock_table_name" {
  description = "DynamoDB table name for Terraform state locking."
  type        = string
  default     = "authproxy-tf-locks"
}

variable "tags" {
  description = "Tags applied to all resources in this module."
  type        = map(string)
  default = {
    Project   = "authproxy"
    ManagedBy = "terraform"
    Module    = "bootstrap"
  }
}
