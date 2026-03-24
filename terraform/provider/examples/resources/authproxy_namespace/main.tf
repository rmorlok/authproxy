resource "authproxy_namespace" "production" {
  path = "root.production"
  labels = {
    env  = "production"
    team = "platform"
  }
}

resource "authproxy_namespace" "staging" {
  path = "root.staging"
  labels = {
    env = "staging"
  }
}
