resource "authproxy_connector" "gmail" {
  namespace = "root.production"

  definition = jsonencode({
    display_name = "Gmail"
    description  = "Google Gmail integration"
    auth = {
      type          = "oauth2"
      client_id     = var.gmail_client_id
      client_secret = var.gmail_client_secret
      auth_url      = "https://accounts.google.com/o/oauth2/v2/auth"
      token_url     = "https://oauth2.googleapis.com/token"
      scopes        = ["https://www.googleapis.com/auth/gmail.readonly"]
    }
  })

  labels = {
    service = "google"
    type    = "email"
  }
}

# Draft connector for staging/review
resource "authproxy_connector" "gmail_staging" {
  namespace = "root.staging"
  publish   = false

  definition = jsonencode({
    display_name = "Gmail (Staging)"
    description  = "Gmail connector under review"
    auth = {
      type = "no_auth"
    }
  })
}
