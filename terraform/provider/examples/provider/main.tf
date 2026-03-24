terraform {
  required_providers {
    authproxy = {
      source = "registry.terraform.io/rmorlok/authproxy"
    }
  }
}

provider "authproxy" {
  endpoint     = "http://localhost:8082"
  bearer_token = var.authproxy_bearer_token
}

variable "authproxy_bearer_token" {
  type      = string
  sensitive = true
}
