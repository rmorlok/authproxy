//go:build integration

package terraform

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNamespace_basic(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ns"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_namespace.test", "path", "root.tf-test-ns"),
					resource.TestCheckResourceAttrSet("authproxy_namespace.test", "state"),
					resource.TestCheckResourceAttrSet("authproxy_namespace.test", "created_at"),
					resource.TestCheckResourceAttrSet("authproxy_namespace.test", "updated_at"),
				),
			},
			// Import
			{
				ResourceName:                         "authproxy_namespace.test",
				ImportState:                          true,
				ImportStateVerify:                     true,
				ImportStateId:                         "root.tf-test-ns",
				ImportStateVerifyIdentifierAttribute:  "path",
			},
			// Update labels
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ns"
  labels = {
    env = "test"
    team = "platform"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_namespace.test", "path", "root.tf-test-ns"),
					resource.TestCheckResourceAttr("authproxy_namespace.test", "labels.env", "test"),
					resource.TestCheckResourceAttr("authproxy_namespace.test", "labels.team", "platform"),
				),
			},
		},
	})
}

func TestAccNamespace_annotations(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Step 1: Create with annotations
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ns-annot"
  annotations = {
    description = "A namespace with annotations"
    owner       = "team-infra"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_namespace.test", "path", "root.tf-test-ns-annot"),
					resource.TestCheckResourceAttr("authproxy_namespace.test", "annotations.description", "A namespace with annotations"),
					resource.TestCheckResourceAttr("authproxy_namespace.test", "annotations.owner", "team-infra"),
				),
			},
			// Step 2: Update annotations
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ns-annot"
  annotations = {
    description = "Updated namespace description"
    region      = "us-west-2"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_namespace.test", "annotations.description", "Updated namespace description"),
					resource.TestCheckResourceAttr("authproxy_namespace.test", "annotations.region", "us-west-2"),
				),
			},
			// Step 3: Use both labels and annotations together
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ns-annot"
  labels = {
    env = "staging"
  }
  annotations = {
    description = "Namespace with both labels and annotations"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_namespace.test", "labels.env", "staging"),
					resource.TestCheckResourceAttr("authproxy_namespace.test", "annotations.description", "Namespace with both labels and annotations"),
				),
			},
		},
	})
}

func TestAccNamespaceDataSource(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerCfg + fmt.Sprintf(`
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ns-ds"
  labels = {
    purpose = "datasource-test"
  }
}

data "authproxy_namespace" "test" {
  path = authproxy_namespace.test.path
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.authproxy_namespace.test", "path", "root.tf-test-ns-ds"),
					resource.TestCheckResourceAttrSet("data.authproxy_namespace.test", "state"),
					resource.TestCheckResourceAttrSet("data.authproxy_namespace.test", "created_at"),
				),
			},
		},
	})
}
