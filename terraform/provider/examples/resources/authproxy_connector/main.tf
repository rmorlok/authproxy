resource "authproxy_connector" "gmail" {
  namespace = "root.production"

  definition = jsonencode({
    displayName = "Gmail"
    description = "Google Gmail integration"
    auth = {
      type         = "OAuth2"
      clientId     = var.gmail_client_id
      clientSecret = var.gmail_client_secret
      authorization = {
        endpoint = "https://accounts.google.com/o/oauth2/v2/auth"
      }
      token = {
        endpoint = "https://oauth2.googleapis.com/token"
      }
      scopes = [{
        id     = "https://www.googleapis.com/auth/gmail.readonly"
        reason = "Read Gmail messages"
      }]
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
    displayName = "Gmail (Staging)"
    description = "Gmail connector under review"
    auth = {
      type = "no-auth"
    }
  })
}
