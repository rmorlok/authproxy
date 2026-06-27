//go:build integration

package terraform

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccKey_basic(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-key"
}

resource "authproxy_key" "test" {
  namespace = authproxy_namespace.test.path
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_key.test", "id"),
					resource.TestCheckResourceAttr("authproxy_key.test", "namespace", "root.tf-test-key"),
					resource.TestCheckResourceAttrSet("authproxy_key.test", "state"),
					resource.TestCheckResourceAttrSet("authproxy_key.test", "created_at"),
				),
			},
			// Import
			{
				ResourceName:      "authproxy_key.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update labels
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-key"
}

resource "authproxy_key" "test" {
  namespace = authproxy_namespace.test.path
  labels = {
    env = "test"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_key.test", "labels.env", "test"),
				),
			},
		},
	})
}

func TestAccKey_annotations(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Step 1: Create with annotations
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-key-annot"
}

resource "authproxy_key" "test" {
  namespace = authproxy_namespace.test.path
  annotations = {
    description = "Primary key"
    rotation    = "90d"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_key.test", "id"),
					resource.TestCheckResourceAttr("authproxy_key.test", "annotations.description", "Primary key"),
					resource.TestCheckResourceAttr("authproxy_key.test", "annotations.rotation", "90d"),
				),
			},
			// Step 2: Update annotations
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-key-annot"
}

resource "authproxy_key" "test" {
  namespace = authproxy_namespace.test.path
  annotations = {
    description = "Updated key description"
    managed-by  = "terraform"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_key.test", "annotations.description", "Updated key description"),
					resource.TestCheckResourceAttr("authproxy_key.test", "annotations.managed-by", "terraform"),
				),
			},
			// Step 3: Use both labels and annotations together
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-key-annot"
}

resource "authproxy_key" "test" {
  namespace = authproxy_namespace.test.path
  labels = {
    env = "test"
  }
  annotations = {
    description = "Key with both labels and annotations"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_key.test", "labels.env", "test"),
					resource.TestCheckResourceAttr("authproxy_key.test", "annotations.description", "Key with both labels and annotations"),
				),
			},
		},
	})
}

func TestAccKeyDataSource(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-key-ds"
}

resource "authproxy_key" "test" {
  namespace = authproxy_namespace.test.path
}

data "authproxy_key" "test" {
  id = authproxy_key.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.authproxy_key.test", "id"),
					resource.TestCheckResourceAttr("data.authproxy_key.test", "namespace", "root.tf-test-key-ds"),
					resource.TestCheckResourceAttrSet("data.authproxy_key.test", "state"),
				),
			},
		},
	})
}
