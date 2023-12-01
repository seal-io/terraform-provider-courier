package courier

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seal-io/terraform-provider-courier/pkg/target"
)

var _ datasource.DataSource = (*DataSourceTarget)(nil)

type (
	DataSourceTarget struct {
		Host     DataSourceTargetHost `tfsdk:"host"`
		Timeouts timeouts.Value       `tfsdk:"timeouts"`

		OS      types.String `tfsdk:"os"`
		Arch    types.String `tfsdk:"arch"`
		Version types.String `tfsdk:"version"`
	}

	DataSourceTargetHost struct {
		Address  types.String                `tfsdk:"address"`
		Authn    DataSourceTargetHostAuthn   `tfsdk:"authn"`
		Insecure types.Bool                  `tfsdk:"insecure"`
		Proxies  []DataSourceTargetHostProxy `tfsdk:"proxies"`
	}

	DataSourceTargetHostAuthn struct {
		Type   types.String `tfsdk:"type"`
		User   types.String `tfsdk:"user"`
		Secret types.String `tfsdk:"secret"`
		Agent  types.Bool   `tfsdk:"agent"`
	}

	DataSourceTargetHostProxy struct {
		Address  types.String                   `tfsdk:"address"`
		Authn    DataSourceTargetHostProxyAuthn `tfsdk:"authn"`
		Insecure types.Bool                     `tfsdk:"insecure"`
	}

	DataSourceTargetHostProxyAuthn struct {
		Type   types.String `tfsdk:"type"`
		User   types.String `tfsdk:"user"`
		Secret types.String `tfsdk:"secret"`
	}
)

func NewDataSourceTarget() datasource.DataSource {
	return &DataSourceTarget{}
}

func (r *DataSourceTarget) State(
	ctx context.Context,
) diag.Diagnostics {
	var diags diag.Diagnostics

	h, err := r.Host.Reflect(ctx)
	if err != nil {
		diags.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("host"),
			"Invalid Host",
			fmt.Sprintf("Cannot reflect from host: %v", err),
		))

		return diags
	}

	s, err := h.State(ctx)
	if err != nil {
		diags.Append(diag.NewWarningDiagnostic(
			"Unobservable Host",
			fmt.Sprintf("Cannot state from address %s: %v",
				r.Host.Address.ValueString(), err),
		))
	}

	if s.Accessible {
		r.OS = types.StringValue(s.OS)
		r.Arch = types.StringValue(s.Arch)
		r.Version = types.StringValue(s.Version)
	}

	return diags
}

func (r DataSourceTargetHost) Reflect(
	_ context.Context,
) (target.Host, error) {
	opts := target.HostOptions{
		HostOption: target.HostOption{
			Address: r.Address.ValueString(),
			Authn: target.HostOptionAuthn{
				Type:   r.Authn.Type.ValueString(),
				User:   r.Authn.User.ValueString(),
				Secret: r.Authn.Secret.ValueString(),
				Agent:  r.Authn.Agent.ValueBool(),
			},
			Insecure: r.Insecure.ValueBool(),
		},
	}

	opts.Proxies = make([]target.HostOption, 0, len(r.Proxies))
	for i := range r.Proxies {
		p := r.Proxies[i]
		opts.Proxies = append(opts.Proxies,
			target.HostOption{
				Address: p.Address.ValueString(),
				Authn: target.HostOptionAuthn{
					Type:   p.Authn.Type.ValueString(),
					User:   p.Authn.User.ValueString(),
					Secret: p.Authn.Secret.ValueString(),
				},
				Insecure: p.Insecure.ValueBool(),
			})
	}

	return target.NewHost(opts)
}

func (r *DataSourceTarget) Metadata(
	ctx context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = strings.Join(
		[]string{req.ProviderTypeName, "target"},
		"_",
	)
}

func (r *DataSourceTarget) Schema(
	ctx context.Context,
	req datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: `Specify the target to deploy.`,
		Attributes: map[string]schema.Attribute{
			"host": schema.SingleNestedAttribute{
				Required:    true,
				Description: `Specify the target to access.`,
				Attributes: map[string]schema.Attribute{
					"address": schema.StringAttribute{
						Required: true,
						Description: `The address to access the target, 
in the form of [schema://](ip|dns)[:port].`,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"authn": schema.SingleNestedAttribute{
						Required:    true,
						Description: `The authentication for accessing the host.`,
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Required:    true,
								Description: `The type to access the target, either "ssh" or "winrm".`,
								Validators: []validator.String{
									stringvalidator.OneOf("ssh", "winrm"),
								},
							},
							"user": schema.StringAttribute{
								Optional:    true,
								Description: `The user to authenticate when accessing the target.`,
							},
							"secret": schema.StringAttribute{
								Optional: true,
								Description: `The secret to authenticate when accessing the target, 
either password or private key.`,
								Sensitive: true,
							},
							"agent": schema.BoolAttribute{
								Optional: true,
								Computed: true,
								Description: `Specify to access the target with agent,
either SSH agent if type is "ssh" or NTLM if type is "winrm".`,
							},
						},
					},
					"insecure": schema.BoolAttribute{
						Optional:    true,
						Description: `Specify to access the target with insecure mode.`,
					},
					"proxies": schema.ListNestedAttribute{
						Optional: true,
						Description: `The proxies before accessing the target, 
either a bastion host or a jump host.`,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"address": schema.StringAttribute{
									Required: true,
									Description: `The address to access the proxy, 
in the form of [schema://](ip|dns)[:port].`,
									Validators: []validator.String{
										stringvalidator.LengthAtLeast(1),
									},
								},
								"authn": schema.SingleNestedAttribute{
									Required:    true,
									Description: `The authentication for accessing the proxy.`,
									Attributes: map[string]schema.Attribute{
										"type": schema.StringAttribute{
											Required:    true,
											Description: `The type to access the proxy, either "ssh" or "proxy".`,
											Validators: []validator.String{
												stringvalidator.OneOf(
													"ssh",
													"proxy",
												),
											},
										},
										"user": schema.StringAttribute{
											Optional:    true,
											Description: `The user to authenticate when accessing the proxy.`,
										},
										"secret": schema.StringAttribute{
											Optional: true,
											Description: `The secret to authenticate when accessing the proxy, 
either password or private key.`,
											Sensitive: true,
										},
									},
								},
								"insecure": schema.BoolAttribute{
									Optional:    true,
									Description: `Specify to access the target with insecure mode.`,
								},
							},
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
			"os": schema.StringAttribute{
				Computed:    true,
				Description: `Observes the operating system of the target.`,
			},
			"arch": schema.StringAttribute{
				Computed:    true,
				Description: `Observes the architecture of the target.`,
			},
			"version": schema.StringAttribute{
				Computed:    true,
				Description: `Observes the kernel version of the target.`,
			},
		},
	}
}

func (r *DataSourceTarget) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var plan DataSourceTarget

	resp.Diagnostics.Append(req.Config.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	{
		// Get Timeout.
		timeout, diags := plan.Timeouts.Read(ctx, 10*time.Minute)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// State.
		resp.Diagnostics.Append(plan.State(ctx)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
