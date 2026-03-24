//go:build integration

package terraform

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/terraform/provider/authproxyprovider"
)

// testProviderConfig returns the HCL provider configuration block for a test environment.
func testProviderConfig(env *helpers.IntegrationTestEnv) string {
	return `
provider "authproxy" {
  endpoint     = "` + env.ServerURL + `"
  bearer_token = "` + env.BearerToken + `"
}
`
}

// testSetup creates an integration test environment with the admin API and HTTP server.
func testSetup(t *testing.T) *helpers.IntegrationTestEnv {
	t.Helper()
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:         helpers.ServiceTypeAdminAPI,
		StartHTTPServer: true,
	})
	t.Cleanup(env.Cleanup)
	return env
}

// testAccProtoV6ProviderFactories returns the provider factories for acceptance tests.
func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"authproxy": providerserver.NewProtocol6WithError(authproxyprovider.New("test")()),
	}
}

// testAccCheck runs a Terraform acceptance test with the given steps.
func testAccCheck(t *testing.T, env *helpers.IntegrationTestEnv, steps []resource.TestStep) {
	t.Helper()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps:                    steps,
	})
}
