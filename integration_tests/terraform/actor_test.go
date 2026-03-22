//go:build integration

package terraform

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccActor_basic(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-actor"
}

resource "authproxy_actor" "test" {
  namespace   = authproxy_namespace.test.path
  external_id = "user-123"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_actor.test", "id"),
					resource.TestCheckResourceAttr("authproxy_actor.test", "namespace", "root.tf-test-actor"),
					resource.TestCheckResourceAttr("authproxy_actor.test", "external_id", "user-123"),
					resource.TestCheckResourceAttrSet("authproxy_actor.test", "created_at"),
				),
			},
			// Import
			{
				ResourceName:      "authproxy_actor.test",
				ImportState:        true,
				ImportStateVerify:  true,
			},
			// Update labels
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-actor"
}

resource "authproxy_actor" "test" {
  namespace   = authproxy_namespace.test.path
  external_id = "user-123"
  labels = {
    role = "admin"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_actor.test", "labels.role", "admin"),
				),
			},
		},
	})
}

func TestAccActorDataSource(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-actor-ds"
}

resource "authproxy_actor" "test" {
  namespace   = authproxy_namespace.test.path
  external_id = "user-ds-456"
}

data "authproxy_actor" "test" {
  id = authproxy_actor.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.authproxy_actor.test", "id"),
					resource.TestCheckResourceAttr("data.authproxy_actor.test", "namespace", "root.tf-test-actor-ds"),
					resource.TestCheckResourceAttr("data.authproxy_actor.test", "external_id", "user-ds-456"),
				),
			},
		},
	})
}
