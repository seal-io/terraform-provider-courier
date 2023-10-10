package courier

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSourceRuntime_basic(t *testing.T) {
	resourceName := "data.courier_runtime.test"

	resource.Test(t, resource.TestCase{
		IDRefreshIgnore:          []string{resourceName},
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "courier_runtime" "test" {
	class = "tomcat"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"class",
						"tomcat",
					),
					resource.TestCheckResourceAttrSet(
						resourceName,
						"classes.tomcat.#",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"id",
						"0efd5dbf5f4ebc3d",
					),
				),
			},
			{
				Config: `
data "courier_runtime" "test" {
    source = "https://github.com/seal-io/terraform-provider-courier//pkg/runtime/source_builtin?ref=v0.0.1"
	class  = "tomcat"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"class",
						"tomcat",
					),
					resource.TestCheckResourceAttrSet(
						resourceName,
						"classes.tomcat.#",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"id",
						"22505c31b3d5d641",
					),
				),
			},
		},
	})
}
