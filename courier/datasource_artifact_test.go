package courier

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSourceArtifact_basic(t *testing.T) {
	resourceName := "data.courier_artifact.test"

	resource.Test(t, resource.TestCase{
		IDRefreshIgnore:          []string{resourceName},
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
variable "uri" {
  type = string
}

data "courier_artifact" "test" {
  refer  = {
	uri      = var.uri
	insecure = true
  }

  ports = ["8080"]
}`,
				ConfigVariables: config.Variables{
					"uri": config.StringVariable(
						"https://tomcat.apache.org/tomcat-7.0-doc/appdev/sample/sample.war",
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"refer.uri",
						"https://tomcat.apache.org/tomcat-7.0-doc/appdev/sample/sample.war",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"ports.0",
						"8080",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"digest",
						"sha256:89b33caa5bf4cfd235f060c396cb1a5acb2734a1366db325676f48c5f5ed92e5",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"length",
						"4606",
					),
				),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		IDRefreshIgnore:          []string{resourceName},
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "courier_artifact" "test" {
  refer = {
    uri = "nginx:1.25.2"
  }

  command = "nginx-debug -g 'daemon off;'"
  ports = ["80","443"]
  envs = {
    x = "y"
  }
  volumes = [
    "/x", "/y"
  ]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"refer.uri",
						"nginx:1.25.2",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"command",
						"nginx-debug -g 'daemon off;'",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"ports.1",
						"443",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"envs.x",
						"y",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"volumes.1",
						"/y",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"digest",
						"sha256:32da30332506740a2f7c34d5dc70467b7f14ec67d912703568daff790ab3f755",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"length",
						"1862",
					),
				),
			},
		},
	})
}
