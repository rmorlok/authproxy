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
  default     = "1.34"
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
  description = "Instance types for the managed node group."
  type        = list(string)
  default     = ["t3.medium"]
}

variable "node_group_min_size" {
  description = "Min node count for the managed node group."
  type        = number
  default     = 1
}

variable "node_group_desired_size" {
  description = "Desired node count for the managed node group at bootstrap time."
  type        = number
  default     = 2
}

variable "node_group_max_size" {
  description = "Max node count for the managed node group (caps autoscaling)."
  type        = number
  default     = 4
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
