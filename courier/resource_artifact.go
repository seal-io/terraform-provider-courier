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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/seal-io/terraform-provider-courier/pkg/artifact"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

var _ resource.ResourceWithConfigure = (*ResourceArtifact)(nil)

type (
	ResourceArtifact struct {
		_ProviderConfig ProviderConfig

		Refer    ResourceArtifactRefer `tfsdk:"refer"`
		Runtime  types.String          `tfsdk:"runtime"`
		Command  types.String          `tfsdk:"command"`
		Ports    []types.Int64         `tfsdk:"ports"`
		Envs     types.Map             `tfsdk:"envs"`
		Volumes  []types.String        `tfsdk:"volumes"`
		Timeouts timeouts.Value        `tfsdk:"timeouts"`

		ID     types.String `tfsdk:"id"`
		Digest types.String `tfsdk:"digest"`
		Type   types.String `tfsdk:"type"`
		Length types.Int64  `tfsdk:"length"`
	}

	ResourceArtifactRefer struct {
		URI      types.String                `tfsdk:"uri"`
		Authn    *ResourceArtifactReferAuthn `tfsdk:"authn"`
		Insecure types.Bool                  `tfsdk:"insecure"`
	}

	ResourceArtifactReferAuthn struct {
		Type   types.String `tfsdk:"type"`
		User   types.String `tfsdk:"user"`
		Secret types.String `tfsdk:"secret"`
	}
)

func NewResourceArtifact() resource.Resource {
	return &ResourceArtifact{}
}

func (r ResourceArtifact) Equal(l ResourceArtifact) bool {
	return r.Refer.Equal(l.Refer) && r.Runtime.Equal(l.Runtime)
}

func (r ResourceArtifact) Hash() string {
	return strx.Sum(r.Refer.Hash(), r.Runtime.ValueString())
}

func (r *ResourceArtifact) State(ctx context.Context) (diags diag.Diagnostics) {
	p, err := r.Refer.Reflect(ctx)
	if err != nil {
		diags.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("refer"),
			"Invalid Refer",
			fmt.Sprintf("Cannot reflect from refer: %v", err),
		))

		return
	}

	s, err := p.State(ctx)
	if err != nil {
		diags.Append(diag.NewWarningDiagnostic(
			"Unobservable Refer",
			fmt.Sprintf("Cannot state from uri %s: %v",
				r.Refer.URI.ValueString(), err),
		))
	}

	if s.Accessible {
		r.Digest = types.StringValue(s.Digest)
		r.Type = types.StringValue(s.Type)
		r.Length = types.Int64Value(s.Length)
	}

	return
}

func (r ResourceArtifactRefer) Equal(l ResourceArtifactRefer) bool {
	return r.URI.Equal(l.URI)
}

func (r ResourceArtifactRefer) Hash() string {
	return strx.Sum(r.URI.ValueString())
}

func (r ResourceArtifactRefer) Reflect(_ context.Context) (artifact.Refer, error) {
	opts := artifact.ReferOptions{
		URI:      r.URI.ValueString(),
		Insecure: r.Insecure.ValueBool(),
	}

	if au := r.Authn; au != nil {
		opts.Authn = artifact.ReferOptionAuthn{
			Type:   au.Type.ValueString(),
			User:   au.User.ValueString(),
			Secret: au.Secret.ValueString(),
		}
	}

	return artifact.NewPackage(opts)
}

func (r ResourceArtifact) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = strings.Join([]string{req.ProviderTypeName, "artifact"}, "_")
}

func (r ResourceArtifact) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `Specify the artifact to deploy.`,
		Attributes: map[string]schema.Attribute{
			"refer": schema.SingleNestedAttribute{
				Required:    true,
				Description: `The reference of the artifact.`,
				Attributes: map[string]schema.Attribute{
					"uri": schema.StringAttribute{
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
						Required:    true,
						Description: `The reference to pull the artifact.`,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"authn": schema.SingleNestedAttribute{
						Optional:    true,
						Description: `The authentication for pulling the artifact.`,
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Optional:    true,
								Computed:    true,
								Default:     stringdefault.StaticString("basic"),
								Description: `The type for authentication, either "basic" or "bearer".`,
								Validators: []validator.String{
									stringvalidator.OneOf("basic", "bearer"),
								},
							},
							"user": schema.StringAttribute{
								Optional:    true,
								Computed:    true,
								Default:     stringdefault.StaticString(""),
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
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: `Specify to pull the artifact with insecure mode.`,
					},
				},
			},
			"runtime": schema.StringAttribute{
				Required:    true,
				Description: `The runtime of the artifact.`,
			},
			"command": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Description: `The command to start the artifact.`,
			},
			"ports": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				Default:     listdefault.StaticValue(basetypes.NewListNull(types.Int64Type)),
				Description: `The ports of the artifact.`,
				ElementType: types.Int64Type,
			},
			"envs": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Default:     mapdefault.StaticValue(basetypes.NewMapNull(types.StringType)),
				Description: `The environment variables of the artifact.`,
				ElementType: types.StringType,
			},
			"volumes": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				Default:     listdefault.StaticValue(basetypes.NewListNull(types.StringType)),
				Description: `The volumes of the artifact.`,
				ElementType: types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
			}),
			"id": schema.StringAttribute{
				Computed:    true,
				Description: `The ID of the artifact.`,
			},
			"digest": schema.StringAttribute{
				Computed: true,
				Description: `Observes the digest of the artifact, 
in form of algorithm:checksum.`,
			},
			"type": schema.StringAttribute{
				Computed:    true,
				Description: `Observes the type of the artifact.`,
			},
			"length": schema.Int64Attribute{
				Computed: true,
				Description: `Observes the content length of the artifact,
may not be available for all types of artifact.`,
			},
		},
	}
}

func (r ResourceArtifact) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if _, ok := ctx.Deadline(); !ok {
		timeout, diags := r.Timeouts.Create(ctx, 30*time.Minute)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var plan ResourceArtifact

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan._ProviderConfig = r._ProviderConfig
	plan.ID = types.StringValue(plan.Hash())

	// Validate.
	if !r._ProviderConfig.RuntimeClasses.HasRuntime(plan.Runtime.ValueString()) {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("runtime"),
			"Unknown Runtime",
			fmt.Sprintf("Cannot find the runtime %s from config", plan.Runtime.ValueString()),
		))
	}

	// State.
	resp.Diagnostics.Append(plan.State(ctx)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r ResourceArtifact) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}

func (r ResourceArtifact) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if _, ok := ctx.Deadline(); !ok {
		timeout, diags := r.Timeouts.Update(ctx, 30*time.Minute)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	r.Create(
		ctx,
		resource.CreateRequest{
			Config:       req.Config,
			Plan:         req.Plan,
			ProviderMeta: req.ProviderMeta,
		},
		(*resource.CreateResponse)(resp))
}

func (r ResourceArtifact) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *ResourceArtifact) Configure(
	ctx context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		resp.Diagnostics.Append(diag.NewErrorDiagnostic(
			"Invalid Provider Config",
			"Cannot find provider config",
		))

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
