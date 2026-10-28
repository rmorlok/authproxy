variable "region" {
  description = "AWS region for the cluster and its supporting resources."
  type        = string
  default     = "us-east-1"
}

variable "cluster_name" {
  description = "Name of the EKS cluster."
  type        = string
  default     = "authproxy-eks"
}

variable "kubernetes_version" {
  description = "EKS Kubernetes version."
  type        = string
  default     = "1.36"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.0.0.0/16"
}

variable "az_count" {
  description = "Number of Availability Zones to span. EKS requires at least 2; 3 is the typical sweet spot."
  type        = number
  default     = 3
}

variable "node_instance_types" {
  # t3.large: 2 vCPU / 8 GiB and a 35-pod VPC CNI limit. The demo runs one
  # Deployment per AuthProxy service plus Postgres, ClickHouse, Redis,
  # SeaweedFS, and otel-lgtm; on t3.medium (4 GiB, 17 pods) both nodes hit the
  # pod cap and the data-store node ran out of memory.
  description = "Instance types for the managed node group."
  type        = list(string)
  default     = ["t3.large"]
}

variable "node_group_min_size" {
  # The demo's EBS volumes are zonal and the node group spans az_count AZs.
  # Keeping one node per AZ guarantees a node can always mount each volume,
  # even after node replacement. The EKS module ignores desired_size changes
  # after creation, so the minimum is what actually raises the node count.
  description = "Min node count for the managed node group. Keep at least az_count so every AZ (and its EBS volumes) has a node."
  type        = number
  default     = 3
}

variable "node_group_desired_size" {
  description = "Desired node count for the managed node group at bootstrap time. Ignored by the EKS module after creation; raise node_group_min_size to scale an existing group."
  type        = number
  default     = 3
}

variable "node_group_max_size" {
  description = "Max node count for the managed node group (caps autoscaling)."
  type        = number
  default     = 6
}

variable "domain_name" {
  description = "Public domain. A Route53 hosted zone is created here; delegate the registrar's NS records to it."
  type        = string
  default     = "authproxy.net"
}

variable "github_repository" {
  description = "GitHub repo (`owner/name`) that the OIDC trust policy binds to."
  type        = string
  default     = "rmorlok/authproxy"
}

variable "tags" {
  description = "Tags applied to every resource."
  type        = map(string)
  default = {
    Project   = "authproxy"
    ManagedBy = "terraform"
    Module    = "eks"
  }
}
