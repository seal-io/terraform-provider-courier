package courier

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seal-io/terraform-provider-courier/pkg/runtime"
)

var _ datasource.DataSource = (*DataSourceRuntime)(nil)

type (
	DataSourceRuntime struct {
		Class    types.String            `tfsdk:"class"`
		Source   types.String            `tfsdk:"source"`
		Authn    *DataSourceRuntimeAuthn `tfsdk:"authn"`
		Insecure types.Bool              `tfsdk:"insecure"`
		Timeouts timeouts.Value          `tfsdk:"timeouts"`

		Classes map[string]types.List `tfsdk:"classes"`
	}

	DataSourceRuntimeAuthn struct {
		Type   types.String `tfsdk:"type"`
		User   types.String `tfsdk:"user"`
		Secret types.String `tfsdk:"secret"`
	}
)

func NewDataSourceRuntime() datasource.DataSource {
	return &DataSourceRuntime{}
}

func (r *DataSourceRuntime) Metadata(
	ctx context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = strings.Join(
		[]string{req.ProviderTypeName, "runtime"},
		"_",
	)
}

func (r *DataSourceRuntime) Schema(
	ctx context.Context,
	req datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: `Specify how to run.`,
		Attributes: map[string]schema.Attribute{
			"class": schema.StringAttribute{
				Required:    true,
				Description: `Specify the class of the runtime.`,
			},
			"source": schema.StringAttribute{
				Optional: true,
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
				Description: `The authentication for fetch the runtime.`,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Optional:    true,
						Description: `The type for authentication, either "basic" or "bearer".`,
						Validators: []validator.String{
							stringvalidator.OneOf(
								"basic",
								"bearer",
							),
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
			"timeouts": timeouts.Attributes(ctx),
			"classes": schema.MapAttribute{
				Computed:    true,
				Description: `Observes the classes of the runtime.`,
				ElementType: types.ListType{
					ElemType: types.StringType,
				},
			},
		},
	}
}

func (r *DataSourceRuntime) Reflect(
	ctx context.Context,
) (runtime.Source, error) {
	if r.Source.ValueString() == "" {
		return runtime.BuiltinSource(), nil
	}

	opts := runtime.ExternalSourceOptions{
		Source:   r.Source.ValueString(),
		Insecure: r.Insecure.ValueBool(),
	}
	if au := r.Authn; au != nil {
		opts.Authn = runtime.ExternalSourceOptionAuthn{
			Type:   au.Type.ValueString(),
			User:   au.User.ValueString(),
			Secret: au.Secret.ValueString(),
		}
	}

	return runtime.ExternalSource(ctx, opts)
}

func (r *DataSourceRuntime) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var plan DataSourceRuntime

	resp.Diagnostics.Append(req.Config.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var src runtime.Source
	{
		// Get Timeout.
		timeout, diags := plan.Timeouts.Read(ctx, 10*time.Minute)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Reflect.
		var err error
		src, err = plan.Reflect(ctx)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("source"),
				"Invalid Source",
				fmt.Sprintf("Cannot get source: %v", err),
			)

			return
		}
	}

	clz, err := runtime.GetClasses(src)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("source"),
			"Invalid Classes",
			fmt.Sprintf("Cannot get classes: %v", err),
		)

		return
	}

	plan.Classes = make(map[string]types.List, len(clz))
	for k := range clz {
		osList := make([]attr.Value, 0, len(clz[k]))
		for v := range clz[k] {
			osList = append(osList, types.StringValue(v))
		}
		plan.Classes[k] = types.ListValueMust(types.StringType, osList)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate.
	if !clz.Has(plan.Class.ValueString()) {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("class"),
			"Unknown Class",
			fmt.Sprintf(
				"Cannot find the runtime class %s",
				plan.Class.ValueString(),
			),
		))
	}
}
