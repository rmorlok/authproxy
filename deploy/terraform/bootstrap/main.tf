provider "aws" {
  region = var.region

  default_tags {
    tags = var.tags
  }
}

data "aws_caller_identity" "current" {}

locals {
  # Bucket names are globally unique; suffix with the account id so a fork
  # of this repo can apply against a different account without clashing.
  state_bucket_name = coalesce(
    var.state_bucket_name != "" ? var.state_bucket_name : null,
    "authproxy-tf-state-${data.aws_caller_identity.current.account_id}"
  )
}

# --- S3 state bucket -----------------------------------------------------

resource "aws_s3_bucket" "state" {
  bucket = local.state_bucket_name

  # The state bucket is intentionally non-destroyable from this module —
  # tearing it down would orphan the eks/ module's state. To delete, empty
  # the bucket manually and remove this resource (or the whole module).
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket = aws_s3_bucket.state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# --- DynamoDB lock table -------------------------------------------------

resource "aws_dynamodb_table" "lock" {
  name         = var.lock_table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  point_in_time_recovery {
    enabled = true
  }
}
