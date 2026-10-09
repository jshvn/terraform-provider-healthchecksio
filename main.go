// The terraform-provider-healthchecksio binary serves the provider to Terraform or
// OpenTofu.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/provider"
)

// version is set by the release build (GoReleaser ldflags).
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "start the provider for a debugger, for example delve")
	flag.Parse()
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/jshvn/healthchecksio",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
