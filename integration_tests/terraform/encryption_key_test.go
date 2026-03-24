//go:build integration

package terraform

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEncryptionKey_basic(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ek"
}

resource "authproxy_encryption_key" "test" {
  namespace = authproxy_namespace.test.path
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_encryption_key.test", "id"),
					resource.TestCheckResourceAttr("authproxy_encryption_key.test", "namespace", "root.tf-test-ek"),
					resource.TestCheckResourceAttrSet("authproxy_encryption_key.test", "state"),
					resource.TestCheckResourceAttrSet("authproxy_encryption_key.test", "created_at"),
				),
			},
			// Import
			{
				ResourceName:      "authproxy_encryption_key.test",
				ImportState:        true,
				ImportStateVerify:  true,
			},
			// Update labels
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ek"
}

resource "authproxy_encryption_key" "test" {
  namespace = authproxy_namespace.test.path
  labels = {
    env = "test"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_encryption_key.test", "labels.env", "test"),
				),
			},
		},
	})
}

func TestAccEncryptionKeyDataSource(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-ek-ds"
}

resource "authproxy_encryption_key" "test" {
  namespace = authproxy_namespace.test.path
}

data "authproxy_encryption_key" "test" {
  id = authproxy_encryption_key.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.authproxy_encryption_key.test", "id"),
					resource.TestCheckResourceAttr("data.authproxy_encryption_key.test", "namespace", "root.tf-test-ek-ds"),
					resource.TestCheckResourceAttrSet("data.authproxy_encryption_key.test", "state"),
				),
			},
		},
	})
}
