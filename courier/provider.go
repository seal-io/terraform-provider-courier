package courier

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seal-io/terraform-provider-courier/pkg/runtime"
	"github.com/seal-io/terraform-provider-courier/utils/version"
)

var _ provider.Provider = (*Provider)(nil)

const (
	ProviderHostname  = "registry.terraform.io"
	ProviderNamespace = "seal-io"
	ProviderType      = "courier"
	ProviderAddress   = ProviderHostname + "/" + ProviderNamespace + "/" + ProviderType
)

type (
	Provider struct {
		Runtime *ProviderRuntime `tfsdk:"runtime"`
	}

	ProviderRuntime struct {
		Source   types.String          `tfsdk:"source"`
		Authn    *ProviderRuntimeAuthn `tfsdk:"authn"`
		Insecure types.Bool            `tfsdk:"insecure"`
	}

	ProviderRuntimeAuthn struct {
		Type   types.String `tfsdk:"type"`
		User   types.String `tfsdk:"user"`
		Secret types.String `tfsdk:"secret"`
	}
)

func NewProvider() provider.Provider {
	return Provider{}
}

func (p Provider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = ProviderType
	resp.Version = version.Version
}

func (p Provider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `Deliver a Web Application artifact to the related Web Server.`,
		Attributes: map[string]schema.Attribute{
			"runtime": schema.SingleNestedAttribute{
				Optional:    true,
				Description: `Define the runtime collection to deploy the artifact.`,
				Attributes: map[string]schema.Attribute{
					"source": schema.StringAttribute{
						Required: true,
						Description: `The source to fetch the runtime, 
only support a git repository at present.

  - For example:
    - https://github.com/foo/bar, clone the HEAD commit of the default branch.
    - https://github.com/foo/bar//subpath, clone the HEAD commit of the default branch, 
      and use the subdirectory.
    - https://github.com/foo/bar?ref=dev, clone the "dev" commit.
  - Comply with the following structure:
` + "    ```" + `
    /tomcat     	 # the name of the runtime.
      /linux         # the os supported by the runtime.
        /service.sh  # the POSIX shell script, must name as service.sh.
          setup
          start
          state
          stop
          cleanup
      /windows
        /service.ps1 # the PowerShell script, must name as service.ps1.
` + "    ```",
					},
					"authn": schema.SingleNestedAttribute{
						Optional:    true,
						Description: `The authentication for pulling the artifact.`,
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Optional:    true,
								Description: `The type for authentication, either "basic" or "bearer".`,
								Validators: []validator.String{
									stringvalidator.OneOf("basic", "bearer"),
								},
							},
							"user": schema.StringAttribute{
								Optional:    true,
								Description: `The user for authentication.`,
							},
							"secret": schema.StringAttribute{
								Required:    true,
								Description: `The secret for authentication, either password or token.`,
								Sensitive:   true,
							},
						},
					},
					"insecure": schema.BoolAttribute{
						Optional:    true,
						Description: `Specify to fetch the runtime with insecure mode.`,
					},
				},
			},
		},
	}
}

type ProviderConfig struct {
	RuntimeSource  fs.FS
	RuntimeClasses runtime.Classes
}

func (p Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	config := p

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if config.Runtime == nil {
		resp.Diagnostics.AddAttributeWarning(
			path.Root("runtime"),
			"Builtin Runtime Source",
			"Using the builtin runtime source",
		)

		src := runtime.BuiltinSource()
		clz, err := runtime.GetClasses(src)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("runtime"),
				"Invalid Builtin Runtime Classes",
				fmt.Sprintf("Cannot get builtin runtime classes: %v", err),
			)

			return
		}

		resp.ResourceData = ProviderConfig{
			RuntimeSource:  src,
			RuntimeClasses: clz,
		}

		return
	}

	opts := runtime.ExternalSourceOptions{
		Source:   config.Runtime.Source.ValueString(),
		Insecure: config.Runtime.Insecure.ValueBool(),
	}

	if au := config.Runtime.Authn; au != nil {
		opts.Authn = runtime.ExternalSourceOptionAuthn{
			Type:   au.Type.ValueString(),
			User:   au.User.ValueString(),
			Secret: au.Secret.ValueString(),
		}
	}

	src, err := runtime.ExternalSource(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("runtime"),
			"External Runtime Source",
			fmt.Sprintf("Cannot use the external runtime source: %v", err),
		)

		return
	}

	clz, err := runtime.GetClasses(src)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("runtime"),
			"Invalid Runtime Classes",
			fmt.Sprintf("Cannot get external runtime classes: %v", err),
		)

		return
	}

	resp.ResourceData = ProviderConfig{
		RuntimeSource:  src,
		RuntimeClasses: clz,
	}
}

func (p Provider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

func (p Provider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewResourceDeployment,
		NewResourceArtifact,
		NewResourceTarget,
	}
}
