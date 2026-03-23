package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/rmorlok/authproxy/terraform/provider/internal/provider"
)

// version is set via ldflags at build time.
var version = "dev"

func main() {
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/rmorlok/authproxy",
	})
	if err != nil {
		log.Fatal(err)
	}
}
