package main

import (
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/seal-io/terraform-provider-courier/courier"
	"github.com/seal-io/terraform-provider-courier/utils/signalx"
)

func main() {
	var debug bool

	flag.BoolVar(
		&debug,
		"debug",
		false,
		"Start provider in stand-alone debug mode.",
	)
	flag.Parse()

	err := providerserver.Serve(
		signalx.Context(),
		courier.NewProvider,
		providerserver.ServeOpts{
			Address: courier.ProviderAddress,
			Debug:   debug,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}
