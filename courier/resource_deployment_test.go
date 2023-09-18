package courier

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccResourceDeployment_basic(t *testing.T) {
	// Start virtual machines.
	ctx := context.TODO()

	mp, err := getMultipass(1)
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
			t.Errorf("failed to stop virtual machines via multipass: %v", err)
		}
	}()

	// Test target.
	priKey, hosts, err := mp.GetEndpoints(t, ctx)
	if err != nil {
		t.Errorf("failed to get credential: %v", err)
		return
	}

	resourceName := "courier_deployment.test"

	resource.Test(t, resource.TestCase{
		IDRefreshName:            resourceName,
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
variable "hosts" {
  type = list(string)
}

variable "secret" {
  type      = string
  sensitive = true
}

resource "courier_target" "test" {
  count = length(var.hosts)
  
  host = {
    address = var.hosts[count.index]
    authn   = {
      type   = "ssh"
      user   = "root"
      secret = var.secret
    }
    insecure = true
  }
}

resource "courier_artifact" "test" {
  refer = {
    uri = "nginx:1.25.2"
  }

  runtime = "docker"
  ports = ["80","443"]
}

resource "courier_deployment" "test" {
  artifact = courier_artifact.test
  targets  = courier_target.test
}
`,
				ConfigVariables: config.Variables{
					"hosts": func() config.Variable {
						r := make([]config.Variable, 0, len(hosts))
						for i := range hosts {
							r = append(r, config.StringVariable(hosts[i]))
						}
						return config.ListVariable(r...)
					}(),
					"secret": config.StringVariable(priKey),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
		},
	})
}

func TestAccResourceDeployment_rolling(t *testing.T) {
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
			t.Errorf("failed to stop virtual machines via multipass: %v", err)
		}
	}()

	// Test target.
	priKey, hosts, err := mp.GetEndpoints(t, ctx)
	if err != nil {
		t.Errorf("failed to get credential: %v", err)
		return
	}

	resourceName := "courier_deployment.test"
	resourceConfig := `
variable "hosts" {
  type = list(string)
}

variable "secret" {
  type      = string
  sensitive = true
}

resource "courier_target" "test" {
  count = length(var.hosts)
  
  host = {
    address = var.hosts[count.index]
    authn   = {
      type   = "ssh"
      user   = "root"
      secret = var.secret
    }
    insecure = true
  }
}

resource "courier_artifact" "test" {
  refer = {
    uri = "https://tomcat.apache.org/tomcat-7.0-doc/appdev/sample/sample.war"
  }

  runtime = "tomcat"
  ports = ["80","443"]
}

resource "courier_deployment" "test" {
  artifact = courier_artifact.test
  targets  = courier_target.test
  strategy = {
    type = "rolling"
  }
}
`

	resource.Test(t, resource.TestCase{
		IDRefreshName:            resourceName,
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: resourceConfig,
				ConfigVariables: config.Variables{
					"hosts": func() config.Variable {
						// Partition hosts.
						r := make([]config.Variable, 0, len(hosts[:2]))
						for i := range hosts[:2] {
							r = append(r, config.StringVariable(hosts[i]))
						}
						return config.ListVariable(r...)
					}(),
					"secret": config.StringVariable(priKey),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				Config: resourceConfig,
				ConfigVariables: config.Variables{
					"hosts": func() config.Variable {
						// Full hosts.
						r := make([]config.Variable, 0, len(hosts))
						for i := range hosts {
							r = append(r, config.StringVariable(hosts[i]))
						}
						return config.ListVariable(r...)
					}(),
					"secret": config.StringVariable(priKey),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				Config: resourceConfig,
				ConfigVariables: config.Variables{
					"hosts": func() config.Variable {
						// Reverse hosts.
						r := make([]config.Variable, 0, len(hosts))
						for i := range hosts {
							r = append(r, config.StringVariable(hosts[len(hosts)-i-1]))
						}
						return config.ListVariable(r...)
					}(),
					"secret": config.StringVariable(priKey),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
		},
	})
}
