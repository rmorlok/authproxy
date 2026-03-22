resource "authproxy_encryption_key" "production" {
  namespace = authproxy_namespace.production.path
  labels = {
    purpose = "token-encryption"
  }
}
