package courier

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/seal-io/terraform-provider-courier/pkg/target"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

var _ resource.ResourceWithConfigure = (*ResourceTarget)(nil)

type (
	ResourceTarget struct {
		_ProviderConfig ProviderConfig

		Host     ResourceTargetHost `tfsdk:"host"`
		Timeouts timeouts.Value     `tfsdk:"timeouts"`

		ID      types.String `tfsdk:"id"`
		OS      types.String `tfsdk:"os"`
		Arch    types.String `tfsdk:"arch"`
		Version types.String `tfsdk:"version"`
	}

	ResourceTargetHost struct {
		Address  types.String              `tfsdk:"address"`
		Authn    ResourceTargetHostAuthn   `tfsdk:"authn"`
		Insecure types.Bool                `tfsdk:"insecure"`
		Proxies  []ResourceTargetHostProxy `tfsdk:"proxies"`
	}

	ResourceTargetHostAuthn struct {
		Type   types.String `tfsdk:"type"`
		User   types.String `tfsdk:"user"`
		Secret types.String `tfsdk:"secret"`
		Agent  types.Bool   `tfsdk:"agent"`
	}

	ResourceTargetHostProxy struct {
		Address  types.String            `tfsdk:"address"`
		Authn    ResourceTargetHostAuthn `tfsdk:"authn"`
		Insecure types.Bool              `tfsdk:"insecure"`
	}

	ResourceTargetHostProxyAuthn struct {
		Type   types.String `tfsdk:"type"`
		User   types.String `tfsdk:"user"`
		Secret types.String `tfsdk:"secret"`
	}
)

func NewResourceTarget() resource.Resource {
	return &ResourceTarget{}
}

func (r *ResourceTarget) Equal(l ResourceTarget) bool {
	return r.Host.Equal(l.Host)
}

func (r *ResourceTarget) Hash() string {
	return r.Host.Hash()
}

func (r *ResourceTarget) State(ctx context.Context) (diags diag.Diagnostics) {
	h, err := r.Host.Reflect(ctx)
	if err != nil {
		diags.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("host"),
			"Invalid Host",
			fmt.Sprintf("Cannot reflect from host: %v", err),
		))

		return
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

	return
}

func (r ResourceTargetHost) Equal(l ResourceTargetHost) bool {
	return r.Address.Equal(l.Address)
}

func (r ResourceTargetHost) Hash() string {
	return strx.Sum(r.Address.ValueString())
}

func (r ResourceTargetHost) Reflect(_ context.Context) (target.Host, error) {
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

func (r ResourceTargetHostProxy) Equal(l ResourceTargetHostProxy) bool {
	return r.Address.Equal(l.Address)
}

func (r ResourceTargetHostProxy) Hash() string {
	return strx.Sum(r.Address.ValueString())
}

func (r *ResourceTarget) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = strings.Join([]string{req.ProviderTypeName, "target"}, "_")
}

func (r *ResourceTarget) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `Specify the target to deploy.`,
		Attributes: map[string]schema.Attribute{
			"host": schema.SingleNestedAttribute{
				Required:    true,
				Description: `Specify the target to access.`,
				Attributes: map[string]schema.Attribute{
					"address": schema.StringAttribute{
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
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
								Optional:    true,
								Computed:    true,
								Default:     stringdefault.StaticString("ssh"),
								Description: `The type to access the target, either "ssh" or "winrm".`,
								Validators: []validator.String{
									stringvalidator.OneOf("ssh", "winrm"),
								},
							},
							"user": schema.StringAttribute{
								Optional:    true,
								Computed:    true,
								Default:     stringdefault.StaticString("root"),
								Description: `The user to authenticate when accessing the target.`,
							},
							"secret": schema.StringAttribute{
								Optional: true,
								Computed: true,
								Default:  stringdefault.StaticString(""),
								Description: `The secret to authenticate when accessing the target, 
either password or private key.`,
								Sensitive: true,
							},
							"agent": schema.BoolAttribute{
								Optional: true,
								Computed: true,
								Default:  booldefault.StaticBool(false),
								Description: `Specify to access the target with agent,
either SSH agent if type is "ssh" or NTLM if type is "winrm".`,
							},
						},
					},
					"insecure": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
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
											Optional:    true,
											Computed:    true,
											Default:     stringdefault.StaticString("proxy"),
											Description: `The type to access the proxy, either "ssh" or "proxy".`,
											Validators: []validator.String{
												stringvalidator.OneOf("ssh", "proxy"),
											},
										},
										"user": schema.StringAttribute{
											Optional:    true,
											Computed:    true,
											Default:     stringdefault.StaticString(""),
											Description: `The user to authenticate when accessing the proxy.`,
										},
										"secret": schema.StringAttribute{
											Optional: true,
											Computed: true,
											Default:  stringdefault.StaticString(""),
											Description: `The secret to authenticate when accessing the proxy, 
either password or private key.`,
											Sensitive: true,
										},
									},
								},
								"insecure": schema.BoolAttribute{
									Optional:    true,
									Computed:    true,
									Default:     booldefault.StaticBool(false),
									Description: `Specify to access the target with insecure mode.`,
								},
							},
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
			}),
			"id": schema.StringAttribute{
				Computed:    true,
				Description: `The ID of the target.`,
			},
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

func (r *ResourceTarget) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceTarget

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan._ProviderConfig = r._ProviderConfig
	plan.ID = types.StringValue(plan.Hash())

	{
		// Get Timeout.
		timeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
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

func (r *ResourceTarget) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}

func (r *ResourceTarget) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ResourceTarget

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan._ProviderConfig = r._ProviderConfig
	plan.ID = types.StringValue(plan.Hash())

	if !plan.Equal(state) {
		tflog.Debug(ctx, "Changed, stating again...")

		// Get Timeout.
		timeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
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

func (r *ResourceTarget) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *ResourceTarget) Configure(
	ctx context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	var ok bool
	r._ProviderConfig, ok = req.ProviderData.(ProviderConfig)
	if !ok {
		resp.Diagnostics.Append(diag.NewErrorDiagnostic(
			"Invalid Provider Config",
			"Unknown provider config type",
		))
	}
}
