package courier

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSourceTarget_basic(t *testing.T) {
	// Start virtual machines.
	ctx := context.TODO()

	mp, err := getMultipass(3)
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			t.Errorf("failed to get multipass: %v", err)
			return
		}

		t.Skip("not found multipass")
		return
	}

	err = mp.Start(t, ctx)
	if err != nil {
		t.Errorf("failed to start virtual machines via multipass: %v", err)
		return
	}

	defer func() {
		err = mp.Stop(t, ctx)
		if err != nil {
			t.Errorf(
				"failed to stop virtual machines via multipass: %v",
				err,
			)
		}
	}()

	// Test target.
	priKey, hosts, err := mp.GetEndpoints(t, ctx)
	if err != nil {
		t.Errorf("failed to get credential: %v", err)
		return
	}

	resourceName := "data.courier_target.test"
	resourceConfig := `
variable "hosts" {
  type = list(string)
}

variable "user" {
  type = string
}

variable "secret" {
  type      = string
  sensitive = true
}

data "courier_target" "test" {
  count = length(var.hosts)
  
  host = {
    address = var.hosts[count.index]
    authn   = {
      type   = "ssh"
      user   = var.user
      secret = var.secret
    }
    insecure = true
  }

  timeouts = {
    read = "5m"
  }
}
`

	resource.Test(t, resource.TestCase{
		IDRefreshIgnore:          []string{resourceName},
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: resourceConfig,
				ConfigVariables: config.Variables{
					"hosts": func() config.Variable {
						r := make([]config.Variable, 0, len(hosts))
						for i := range hosts {
							r = append(r, config.StringVariable(hosts[i]))
						}
						return config.ListVariable(r...)
					}(),
					"user":   config.StringVariable("root"),
					"secret": config.StringVariable(priKey),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName+".1",
						"host.address",
						hosts[1],
					),
					resource.TestCheckResourceAttr(
						resourceName+".0",
						"host.authn.type",
						"ssh",
					),
					resource.TestCheckResourceAttr(
						resourceName+".2",
						"host.authn.secret",
						priKey,
					),
					resource.TestCheckResourceAttr(
						resourceName+".1",
						"os",
						"linux",
					),
					resource.TestCheckResourceAttr(
						resourceName+".2",
						"arch",
						"arm64",
					),
				),
			},
			{
				Config: resourceConfig,
				ConfigVariables: config.Variables{
					"hosts": func() config.Variable {
						r := make([]config.Variable, 0, len(hosts))
						for i := range hosts {
							r = append(r, config.StringVariable(hosts[i]))
						}
						return config.ListVariable(r...)
					}(),
					"user":   config.StringVariable("ansible"),
					"secret": config.StringVariable("ansible"),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName+".1",
						"host.address",
						hosts[1],
					),
					resource.TestCheckResourceAttr(
						resourceName+".0",
						"host.authn.type",
						"ssh",
					),
					resource.TestCheckResourceAttr(
						resourceName+".2",
						"host.authn.secret",
						"ansible",
					),
					resource.TestCheckResourceAttr(
						resourceName+".2",
						"os",
						"linux",
					),
					resource.TestCheckResourceAttr(
						resourceName+".1",
						"arch",
						"arm64",
					),
				),
			},
		},
	})
}
