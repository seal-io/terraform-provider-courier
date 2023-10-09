package courier

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seal-io/terraform-provider-courier/pkg/artifact"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

var _ datasource.DataSource = (*DataSourceArtifact)(nil)

type (
	DataSourceArtifact struct {
		Refer    DataSourceArtifactRefer `tfsdk:"refer"`
		Command  types.String            `tfsdk:"command"`
		Ports    []types.Int64           `tfsdk:"ports"`
		Envs     map[string]types.String `tfsdk:"envs"`
		Volumes  []types.String          `tfsdk:"volumes"`
		Timeouts timeouts.Value          `tfsdk:"timeouts"`

		ID     types.String `tfsdk:"id"`
		Digest types.String `tfsdk:"digest"`
		Type   types.String `tfsdk:"type"`
		Length types.Int64  `tfsdk:"length"`
	}

	DataSourceArtifactRefer struct {
		URI      types.String                  `tfsdk:"uri"`
		Authn    *DataSourceArtifactReferAuthn `tfsdk:"authn"`
		Insecure types.Bool                    `tfsdk:"insecure"`
	}

	DataSourceArtifactReferAuthn struct {
		Type   types.String `tfsdk:"type"`
		User   types.String `tfsdk:"user"`
		Secret types.String `tfsdk:"secret"`
	}
)

func NewDataSourceArtifact() datasource.DataSource {
	return &DataSourceArtifact{}
}

func (r *DataSourceArtifact) Equal(l DataSourceArtifact) bool {
	return r.Refer.URI.Equal(l.Refer.URI)
}

func (r *DataSourceArtifact) Hash() string {
	return strx.Sum(r.Refer.URI.ValueString())
}

func (r *DataSourceArtifact) State(
	ctx context.Context,
) diag.Diagnostics {
	var diags diag.Diagnostics

	p, err := r.Refer.Reflect(ctx)
	if err != nil {
		diags.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("refer"),
			"Invalid Refer",
			fmt.Sprintf("Cannot reflect from refer: %v", err),
		))

		return diags
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

	return diags
}

func (r DataSourceArtifactRefer) Reflect(
	_ context.Context,
) (artifact.Refer, error) {
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

func (r *DataSourceArtifact) Metadata(
	ctx context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = strings.Join(
		[]string{req.ProviderTypeName, "artifact"},
		"_",
	)
}

func (r *DataSourceArtifact) Schema(
	ctx context.Context,
	req datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: `Specify the artifact to deploy.`,
		Attributes: map[string]schema.Attribute{
			"refer": schema.SingleNestedAttribute{
				Required:    true,
				Description: `The reference of the artifact.`,
				Attributes: map[string]schema.Attribute{
					"uri": schema.StringAttribute{
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
								Computed:    true,
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
						Description: `Specify to pull the artifact with insecure mode.`,
					},
				},
			},
			"command": schema.StringAttribute{
				Optional:    true,
				Description: `The command to start the artifact.`,
			},
			"ports": schema.ListAttribute{
				Optional:    true,
				Description: `The ports of the artifact.`,
				ElementType: types.Int64Type,
			},
			"envs": schema.MapAttribute{
				Optional:    true,
				Description: `The environment variables of the artifact.`,
				ElementType: types.StringType,
			},
			"volumes": schema.ListAttribute{
				Optional:    true,
				Description: `The volumes of the artifact.`,
				ElementType: types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
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

func (r *DataSourceArtifact) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var plan DataSourceArtifact

	resp.Diagnostics.Append(req.Config.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = types.StringValue(plan.Hash())

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
