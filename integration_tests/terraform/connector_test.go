//go:build integration

package terraform

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// minimalConnectorDefinition is a simple NoAuth connector definition for testing.
const minimalConnectorDefinition = `{
  "display_name": "Test Connector",
  "description": "A test connector for integration tests",
  "auth": {
    "type": "no-auth"
  }
}`

const updatedConnectorDefinition = `{
  "display_name": "Test Connector Updated",
  "description": "An updated test connector",
  "auth": {
    "type": "no-auth"
  }
}`

// TestAccConnector_publishTrue tests: create with publish=true -> version 1 is primary,
// then update definition -> version 2 is primary.
func TestAccConnector_publishTrue(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Step 1: Create connector with publish=true (default)
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "Test Connector"
    description  = "A test connector for integration tests"
    auth = {
      type = "no-auth"
    }
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_connector.test", "id"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "namespace", "root.tf-test-connector"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "version", "1"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "primary"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "publish", "true"),
					resource.TestCheckResourceAttrSet("authproxy_connector.test", "display_name"),
					resource.TestCheckResourceAttrSet("authproxy_connector.test", "created_at"),
				),
			},
			// Step 2: Update definition -> new version 2 becomes primary
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "Test Connector Updated"
    description  = "An updated test connector"
    auth = {
      type = "no-auth"
    }
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_connector.test", "version", "2"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "primary"),
				),
			},
		},
	})
}

// TestAccConnector_publishFalse tests: create with publish=false -> version stays draft.
func TestAccConnector_publishFalse(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Create connector with publish=false -> stays draft
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-draft"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  publish    = false
  definition = jsonencode({
    display_name = "Draft Connector"
    description  = "A draft connector"
    auth = {
      type = "no-auth"
    }
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_connector.test", "id"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "version", "1"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "draft"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "publish", "false"),
				),
			},
		},
	})
}

// TestAccConnector_publishTransition tests: create draft, then change publish to true.
func TestAccConnector_publishTransition(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Step 1: Create as draft
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-transition"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  publish    = false
  definition = jsonencode({
    display_name = "Transitioning Connector"
    description  = "Will be promoted"
    auth = {
      type = "no-auth"
    }
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "draft"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "publish", "false"),
				),
			},
			// Step 2: Change publish to true -> promotes to primary
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-transition"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  publish    = true
  definition = jsonencode({
    display_name = "Transitioning Connector"
    description  = "Will be promoted"
    auth = {
      type = "no-auth"
    }
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "primary"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "publish", "true"),
				),
			},
		},
	})
}

// TestAccConnector_labels tests updating connector labels without changing the definition.
func TestAccConnector_labels(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Step 1: Create with labels
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-labels"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "Labeled Connector"
    description  = "Connector with labels"
    auth = {
      type = "no-auth"
    }
  })
  labels = {
    env = "staging"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_connector.test", "labels.env", "staging"),
				),
			},
			// Step 2: Update labels only
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-labels"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "Labeled Connector"
    description  = "Connector with labels"
    auth = {
      type = "no-auth"
    }
  })
  labels = {
    env  = "production"
    team = "platform"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_connector.test", "labels.env", "production"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "labels.team", "platform"),
					// The API creates a new version for any update on a published connector
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "primary"),
				),
			},
		},
	})
}

// TestAccConnector_annotations tests creating and updating connector annotations.
func TestAccConnector_annotations(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Step 1: Create with annotations
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-annot"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "Annotated Connector"
    description  = "Connector with annotations"
    auth = {
      type = "no-auth"
    }
  })
  annotations = {
    description = "A detailed description of this connector"
    owner       = "team-platform"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("authproxy_connector.test", "id"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "annotations.description", "A detailed description of this connector"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "annotations.owner", "team-platform"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "primary"),
				),
			},
			// Step 2: Update annotations only (no definition change)
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-annot"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "Annotated Connector"
    description  = "Connector with annotations"
    auth = {
      type = "no-auth"
    }
  })
  annotations = {
    description = "Updated description"
    region      = "us-east-1"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_connector.test", "annotations.description", "Updated description"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "annotations.region", "us-east-1"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "primary"),
				),
			},
			// Step 3: Use both labels and annotations together
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-annot"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "Annotated Connector"
    description  = "Connector with annotations"
    auth = {
      type = "no-auth"
    }
  })
  labels = {
    env = "production"
  }
  annotations = {
    description = "Connector with both labels and annotations"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("authproxy_connector.test", "labels.env", "production"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "annotations.description", "Connector with both labels and annotations"),
					resource.TestCheckResourceAttr("authproxy_connector.test", "state", "primary"),
				),
			},
		},
	})
}

// TestAccConnector_import tests importing an existing connector.
func TestAccConnector_import(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-import"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "Import Test Connector"
    description  = "For import testing"
    auth = {
      type = "no-auth"
    }
  })
}
`,
			},
			{
				ResourceName:      "authproxy_connector.test",
				ImportState:        true,
				ImportStateVerify:  true,
			},
		},
	})
}

func TestAccConnectorDataSource(t *testing.T) {
	env := testSetup(t)
	providerCfg := testProviderConfig(env)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerCfg + `
resource "authproxy_namespace" "test" {
  path = "root.tf-test-connector-ds"
}

resource "authproxy_connector" "test" {
  namespace  = authproxy_namespace.test.path
  definition = jsonencode({
    display_name = "DataSource Test Connector"
    description  = "For data source testing"
    auth = {
      type = "no-auth"
    }
  })
}

data "authproxy_connector" "test" {
  id = authproxy_connector.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.authproxy_connector.test", "id"),
					resource.TestCheckResourceAttrSet("data.authproxy_connector.test", "namespace"),
					resource.TestCheckResourceAttrSet("data.authproxy_connector.test", "state"),
					resource.TestCheckResourceAttrSet("data.authproxy_connector.test", "display_name"),
				),
			},
		},
	})
}
