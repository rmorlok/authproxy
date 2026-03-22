package authproxyprovider

import (
	"github.com/hashicorp/terraform-plugin-framework/provider"
	internal "github.com/rmorlok/authproxy/terraform/provider/internal/provider"
)

// New returns a function that creates a new instance of the AuthProxy Terraform provider.
// This is exposed publicly for use in acceptance tests.
func New(version string) func() provider.Provider {
	return internal.New(version)
}
