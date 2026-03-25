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

func TestAccActor_annotations(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Step 1: Create with annotations
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-actor-annot"
}

resource "authproxy_actor" "test" {
  namespace   = authproxy_namespace.test.path
  external_id = "user-annot-123"
  annotations = {
    description = "A test actor with annotations"
    url         = "https://example.com"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_actor.test", "id"),
					resource.TestCheckResourceAttr("authproxy_actor.test", "annotations.description", "A test actor with annotations"),
					resource.TestCheckResourceAttr("authproxy_actor.test", "annotations.url", "https://example.com"),
				),
			},
			// Step 2: Update annotations
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-actor-annot"
}

resource "authproxy_actor" "test" {
  namespace   = authproxy_namespace.test.path
  external_id = "user-annot-123"
  annotations = {
    description = "Updated description"
    team        = "platform"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_actor.test", "annotations.description", "Updated description"),
					resource.TestCheckResourceAttr("authproxy_actor.test", "annotations.team", "platform"),
				),
			},
			// Step 3: Use both labels and annotations together
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-actor-annot"
}

resource "authproxy_actor" "test" {
  namespace   = authproxy_namespace.test.path
  external_id = "user-annot-123"
  labels = {
    env = "test"
  }
  annotations = {
    description = "Actor with both labels and annotations"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_actor.test", "labels.env", "test"),
					resource.TestCheckResourceAttr("authproxy_actor.test", "annotations.description", "Actor with both labels and annotations"),
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
