# Example: per-actor token bucket against Salesforce write traffic.
# 60-token burst, 1 token/sec refill — covers steady ~60 r/min with
# headroom for bursts. Buckets are per-actor + per-team so one team's
# traffic doesn't crowd another's.
resource "authproxy_rate_limit" "team_acme_salesforce_writes" {
  namespace = authproxy_namespace.acme.path
  mode      = "enforce"

  labels = {
    team = "acme"
  }
  annotations = {
    owner = "platform@example.com"
  }

  selector {
    label_selector = "apxy/connector/-/id=salesforce"
    methods        = ["POST", "PATCH", "PUT"]

    path_match {
      kind  = "prefix"
      value = "/services/data/"
    }
  }

  bucket {
    dimensions = ["actor", "labels/team"]
  }

  algorithm {
    token_bucket {
      capacity    = 60
      refill_rate = 1.0
    }
  }
}

# Example: observe-mode rollout. The rule fires and is recorded in the
# request log but never returns 429. Use this to validate a rule's
# match set + counter values at production traffic levels before flipping
# it to enforce.
resource "authproxy_rate_limit" "team_acme_reads_observed" {
  namespace = authproxy_namespace.acme.path
  mode      = "observe"

  selector {
    methods       = ["GET"]
    request_types = ["proxy"]
  }

  bucket {
    dimensions = ["connection"]
  }

  algorithm {
    fixed_window {
      window = "1m"
      limit  = 600
    }
  }
}

# Example: sliding-window counter applied to probes as well as proxy
# traffic. Probes are governed by default so this just makes the
# enrolment explicit.
resource "authproxy_rate_limit" "global_proxy_and_probes" {
  namespace = authproxy_namespace.root.path

  selector {
    request_types = ["proxy", "probe"]
  }

  bucket {
    dimensions = []
  }

  algorithm {
    sliding_window {
      window = "5m"
      limit  = 10000
      mode   = "counter"
    }
  }
}
