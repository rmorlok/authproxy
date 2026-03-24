resource "authproxy_actor" "service_account" {
  namespace   = "root.production"
  external_id = "svc-data-pipeline"
  labels = {
    role = "service-account"
  }
}
