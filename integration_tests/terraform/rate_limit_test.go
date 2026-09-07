//go:build integration

package terraform

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRateLimit_createUpdateImport(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-rate-limit"
}

resource "authproxy_rate_limit" "test" {
  namespace = authproxy_namespace.test.path
  mode      = "observe"

  selector {
    methods = ["GET"]
  }

  bucket {
    dimensions = ["actor"]
  }

  algorithm {
    fixed_window {
      window = "1m"
      limit  = 10
    }
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_rate_limit.test", "id"),
					resource.TestCheckResourceAttr("authproxy_rate_limit.test", "namespace", "root.tf-test-rate-limit"),
					resource.TestCheckResourceAttr("authproxy_rate_limit.test", "mode", "observe"),
					resource.TestCheckResourceAttr("authproxy_rate_limit.test", "selector.methods.0", "GET"),
					resource.TestCheckResourceAttr("authproxy_rate_limit.test", "algorithm.fixed_window.limit", "10"),
				),
			},
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-rate-limit"
}

resource "authproxy_rate_limit" "test" {
  namespace = authproxy_namespace.test.path
  mode      = "enforce"
  labels = {
    team = "platform"
  }

  scope {
    namespace_matcher = "root.tf-test-rate-limit.**"
  }

  selector {
    methods = ["GET", "POST"]
  }

  bucket {
    dimensions = ["actor"]
  }

  algorithm {
    fixed_window {
      window = "1m"
      limit  = 20
    }
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_rate_limit.test", "mode", "enforce"),
					resource.TestCheckResourceAttr("authproxy_rate_limit.test", "labels.team", "platform"),
					resource.TestCheckResourceAttr("authproxy_rate_limit.test", "scope.namespace_matcher", "root.tf-test-rate-limit.**"),
					resource.TestCheckResourceAttr("authproxy_rate_limit.test", "algorithm.fixed_window.limit", "20"),
				),
			},
			{
				ResourceName:      "authproxy_rate_limit.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
