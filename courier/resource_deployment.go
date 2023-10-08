package courier

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/sync/errgroup"

	"github.com/seal-io/terraform-provider-courier/pkg/target"
	"github.com/seal-io/terraform-provider-courier/utils/osx"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

var _ resource.ResourceWithConfigure = (*ResourceDeployment)(nil)

type (
	ResourceDeployment struct {
		_ProviderConfig ProviderConfig

		Artifact ResourceDeploymentArtifact  `tfsdk:"artifact"`
		Targets  []ResourceDeploymentTarget  `tfsdk:"targets"`
		Strategy *ResourceDeploymentStrategy `tfsdk:"strategy"`
		Timeouts timeouts.Value              `tfsdk:"timeouts"`

		ID types.String `tfsdk:"id"`
	}

	ResourceDeploymentArtifact struct {
		ID      types.String          `tfsdk:"id"`
		Refer   ResourceArtifactRefer `tfsdk:"refer"`
		Runtime types.String          `tfsdk:"runtime"`
		Command types.String          `tfsdk:"command"`
		Ports   []types.Int64         `tfsdk:"ports"`
		Envs    types.Map             `tfsdk:"envs"`
		Volumes []types.String        `tfsdk:"volumes"`
		Digest  types.String          `tfsdk:"digest"`
	}

	ResourceDeploymentTarget struct {
		ID   types.String       `tfsdk:"id"`
		Host ResourceTargetHost `tfsdk:"host"`
		OS   types.String       `tfsdk:"os"`
		Arch types.String       `tfsdk:"arch"`
	}

	ResourceDeploymentStrategy struct {
		Type    types.String                       `tfsdk:"type"`
		Rolling *ResourceDeploymentStrategyRolling `tfsdk:"rolling"`
	}

	ResourceDeploymentStrategyRolling struct {
		MaxSurge types.Float64 `tfsdk:"max_surge"`
	}
)

func NewResourceDeployment() resource.Resource {
	return &ResourceDeployment{}
}

func (r *ResourceDeployment) Equal(l ResourceDeployment) bool {
	if !r.Artifact.Equal(l.Artifact) {
		return false
	}

	if len(r.Targets) != len(l.Targets) {
		return false
	} else {
		rtg := append(make([]ResourceDeploymentTarget, 0, len(r.Targets)), r.Targets...)
		ltg := append(make([]ResourceDeploymentTarget, 0, len(l.Targets)), l.Targets...)

		sort.Slice(rtg, func(i, j int) bool {
			return rtg[i].ID.ValueString() < rtg[j].ID.ValueString()
		})
		sort.Slice(ltg, func(i, j int) bool {
			return ltg[i].ID.ValueString() < ltg[j].ID.ValueString()
		})

		for i := range ltg {
			if !rtg[i].Equal(ltg[i]) {
				return false
			}
		}
	}

	if r.Strategy != nil && l.Strategy != nil {
		return r.Strategy.Equal(*l.Strategy)
	} else if r.Strategy != nil || l.Strategy != nil {
		return false
	}

	return true
}

func (r *ResourceDeployment) Hash() string {
	return strx.Sum(r.Artifact.Hash())
}

func (r *ResourceDeployment) Apply(ctx context.Context, prevArt *ResourceDeploymentArtifact) diag.Diagnostics {
	hosts, diags := reflectHosts(ctx, r.Targets)
	if diags.HasError() {
		return diags
	}

	diags.Append(setup(ctx, r._ProviderConfig, r.Artifact, hosts)...)
	if diags.HasError() {
		return diags
	}

	// Rolling.
	if r.Strategy != nil && r.Strategy.Type.ValueString() == "rolling" {
		maxSurge := 0.3

		if r.Strategy.Rolling != nil {
			maxSurge = r.Strategy.Rolling.MaxSurge.ValueFloat64()
			if maxSurge < 0.1 {
				maxSurge = 0.3
			}
		}

		step := int(math.Round(maxSurge * float64(len(hosts))))
		if step == 0 {
			step = 1
		}

		if step != len(hosts) {
			for i, m := 0, len(hosts); i < m; {
				j := i + step
				if j > m {
					j = m
				}

				if prevArt != nil && !prevArt.Equal(r.Artifact) {
					diags.Append(stop(ctx, r.Artifact, hosts[i:j])...)
					if diags.HasError() {
						return diags
					}
				}

				diags.Append(start(ctx, r.Artifact, hosts[i:j])...)
				if diags.HasError() {
					return diags
				}

				i = j
			}

			return diags
		}
	}

	// Recreate.
	if prevArt != nil && !prevArt.Equal(r.Artifact) {
		diags.Append(stop(ctx, r.Artifact, hosts)...)
		if diags.HasError() {
			return diags
		}
	}

	diags.Append(start(ctx, r.Artifact, hosts)...)

	return diags
}

func (r *ResourceDeployment) Release(ctx context.Context) diag.Diagnostics {
	hosts, diags := reflectHosts(ctx, r.Targets)
	if diags.HasError() {
		return diags
	}

	diags.Append(cleanup(ctx, r.Artifact, hosts)...)

	return diags
}

func (r ResourceDeploymentArtifact) Equal(l ResourceDeploymentArtifact) bool {
	return r.Refer.Equal(l.Refer) && r.Runtime.Equal(l.Runtime)
}

func (r ResourceDeploymentArtifact) Hash() string {
	return strx.Sum(r.Refer.Hash(), r.Runtime.ValueString())
}

func (r ResourceDeploymentTarget) Equal(l ResourceDeploymentTarget) bool {
	return r.Host.Equal(l.Host)
}

func (r ResourceDeploymentTarget) Hash() string {
	return r.Host.Hash()
}

func setup(
	ctx context.Context,
	cfg ProviderConfig,
	artifact ResourceDeploymentArtifact,
	hosts []targetHost,
) (diags diag.Diagnostics) {
	// Upload runtime.
	{
		g, ctx := errgroup.WithContext(ctx)

		for i := range hosts {
			h := hosts[i]
			g.Go(func() error {
				// TODO, avoid upload runtime if already exists.
				return h.UploadDirectory(
					ctx,
					cfg.RuntimeSource,
					"/opt/courier/runtime")
			})
		}

		if err := g.Wait(); err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot upload runtime",
				fmt.Sprintf("Cannot upload runtime: %v", err),
			))

			return diags
		}
	}

	// Prepare artifact.
	{
		tmpDir := osx.TempDir("courier-")
		defer func() { _ = os.RemoveAll(tmpDir) }()

		err := os.WriteFile(fmt.Sprintf("%s/command", tmpDir), //nolint:gosec
			[]byte(artifact.Command.ValueString()), 0o666)
		if err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot prepare command",
				fmt.Sprintf("Cannot write command: %v", err),
			))

			return diags
		}

		var (
			ports    = make([]int, 0, len(artifact.Ports))
			portsBuf bytes.Buffer
		)
		for i := range artifact.Ports {
			if artifact.Ports[i].IsNull() || artifact.Ports[i].IsUnknown() {
				continue
			}
			ports = append(ports, int(artifact.Ports[i].ValueInt64()))
		}
		sort.Ints(ports)
		for i := range ports {
			_, _ = fmt.Fprintf(&portsBuf, "%d\n", ports[i])
		}
		err = os.WriteFile(fmt.Sprintf("%s/ports", tmpDir), //nolint:gosec
			portsBuf.Bytes(), 0o666)
		if err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot prepare ports",
				fmt.Sprintf("Cannot prepare ports: %v", err),
			))

			return diags
		}

		var (
			envs    = make([]string, 0, len(artifact.Envs.Elements()))
			envsBuf bytes.Buffer
		)
		for k, v := range artifact.Envs.Elements() {
			if v.IsNull() || v.IsUnknown() {
				envs = append(envs, fmt.Sprintf("%s=", k))
			} else {
				envs = append(envs, fmt.Sprintf("%s=%s", k, v.String()))
			}
		}
		sort.Strings(envs)
		for i := range envs {
			_, _ = fmt.Fprintf(&envsBuf, "%s\n", envs[i])
		}
		err = os.WriteFile(fmt.Sprintf("%s/envs", tmpDir), //nolint:gosec
			envsBuf.Bytes(), 0o666)
		if err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot prepare envs",
				fmt.Sprintf("Cannot prepare envs: %v", err),
			))

			return diags
		}

		var (
			volumes    = make([]string, 0, len(artifact.Volumes))
			volumesBuf bytes.Buffer
		)
		for i := range artifact.Volumes {
			if artifact.Volumes[i].IsNull() || artifact.Volumes[i].IsUnknown() {
				continue
			}
			volumes = append(volumes, artifact.Volumes[i].ValueString())
		}
		sort.Strings(volumes)
		for i := range volumes {
			_, _ = fmt.Fprintf(&volumesBuf, "%s\n", envs[i])
		}
		err = os.WriteFile(fmt.Sprintf("%s/volumes", tmpDir), //nolint:gosec
			volumesBuf.Bytes(), 0o666)
		if err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot prepare volumes",
				fmt.Sprintf("Cannot prepare volumes: %v", err),
			))

			return diags
		}

		g, ctx := errgroup.WithContext(ctx)

		for i := range hosts {
			h := hosts[i]

			g.Go(func() (err error) {
				return h.UploadDirectory(
					ctx,
					os.DirFS(tmpDir),
					fmt.Sprintf("/opt/courier/artifact/%s",
						artifact.ID.ValueString()))
			})
		}

		if err := g.Wait(); err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot upload runtime",
				fmt.Sprintf("Cannot upload runtime: %v", err),
			))

			return diags
		}
	}

	// Upload artifact and setup.
	{
		args := []string{
			"setup",
			artifact.ID.ValueString(),
		}

		args = append(
			args,
			artifact.Refer.URI.ValueString(),
			artifact.Digest.ValueString())

		if au := artifact.Refer.Authn; au != nil {
			args = append(
				args,
				au.Type.ValueString(),
				au.User.ValueString(),
				au.Secret.ValueString(),
			)
		}

		g, ctx := errgroup.WithContext(ctx)

		for i := range hosts {
			h := hosts[i]

			g.Go(func() error {
				if h.OS == "linux" {
					output, err := h.ExecuteWithOutput(
						ctx,
						"chmod",
						"a+x",
						fmt.Sprintf("/opt/courier/runtime/%s/linux/service.sh",
							artifact.Runtime.ValueString()))
					if err != nil {
						tflog.Error(ctx, "cannot change service permission: "+string(output))
						return err
					}
				}

				output, err := h.ExecuteWithOutput(
					ctx,
					fmt.Sprintf("/opt/courier/runtime/%s/%s/service.%s",
						artifact.Runtime.ValueString(),
						h.OS,
						getSuffix(h.OS)),
					args...)
				if err != nil {
					tflog.Error(ctx, "cannot execute service setup: "+string(output))
				}

				return err
			})
		}

		if err := g.Wait(); err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot setup",
				fmt.Sprintf("Cannot setup: %v", err),
			))
		}
	}

	return diags
}

func start(
	ctx context.Context,
	artifact ResourceDeploymentArtifact,
	hosts []targetHost,
) (diags diag.Diagnostics) {
	g, ctx := errgroup.WithContext(ctx)

	for i := range hosts {
		h := hosts[i]

		g.Go(func() error {
			output, err := h.ExecuteWithOutput(
				ctx,
				fmt.Sprintf("/opt/courier/runtime/%s/%s/service.%s",
					artifact.Runtime.ValueString(),
					h.OS,
					getSuffix(h.OS)),
				"start",
				artifact.ID.ValueString(),
			)
			if err != nil {
				tflog.Error(ctx, "cannot execute service start: "+string(output))
			}

			return err
		})
	}

	if err := g.Wait(); err != nil {
		diags.Append(diag.NewErrorDiagnostic(
			"Cannot start",
			fmt.Sprintf("Cannot start: %v", err),
		))
	}

	return diags
}

func stop(
	ctx context.Context,
	artifact ResourceDeploymentArtifact,
	hosts []targetHost,
) (diags diag.Diagnostics) {
	g, ctx := errgroup.WithContext(ctx)

	for i := range hosts {
		h := hosts[i]

		g.Go(func() error {
			output, err := h.ExecuteWithOutput(
				ctx,
				fmt.Sprintf("/opt/courier/runtime/%s/%s/service.%s",
					artifact.Runtime.ValueString(),
					h.OS,
					getSuffix(h.OS)),
				"stop",
				artifact.ID.ValueString(),
			)
			if err != nil {
				tflog.Error(ctx, "cannot execute service stop: "+string(output))
			}

			return err
		})
	}

	if err := g.Wait(); err != nil {
		diags.Append(diag.NewErrorDiagnostic(
			"Cannot stop",
			fmt.Sprintf("Cannot stop: %v", err),
		))
	}

	return diags
}

func cleanup(
	ctx context.Context,
	artifact ResourceDeploymentArtifact,
	hosts []targetHost,
) (diags diag.Diagnostics) {
	g, ctx := errgroup.WithContext(ctx)

	for i := range hosts {
		h := hosts[i]

		g.Go(func() error {
			output, err := h.ExecuteWithOutput(
				ctx,
				fmt.Sprintf("/opt/courier/runtime/%s/%s/service.%s",
					artifact.Runtime.ValueString(),
					h.OS,
					getSuffix(h.OS)),
				"cleanup",
				artifact.ID.ValueString(),
			)
			if err != nil {
				tflog.Error(ctx, "cannot execute service cleanup: "+string(output))
			}

			return err
		})
	}

	if err := g.Wait(); err != nil {
		diags.Append(diag.NewErrorDiagnostic(
			"Cannot cleanup",
			fmt.Sprintf("Cannot cleanup: %v", err),
		))
	}

	return diags
}

func getSuffix(os string) string {
	if os == "windows" {
		return "ps1"
	}
	return "sh"
}

type targetHost struct {
	target.Host

	OS   string
	Arch string
}

func reflectHosts(
	ctx context.Context,
	targets []ResourceDeploymentTarget,
) (hosts []targetHost, diags diag.Diagnostics) {
	hosts = make([]targetHost, 0, len(targets))

	for i := range targets {
		host, err := targets[i].Host.Reflect(ctx)
		if err != nil {
			diags.Append(diag.NewAttributeErrorDiagnostic(
				path.Root("targets").AtListIndex(i).AtName("host"),
				"Invalid Host",
				fmt.Sprintf("Cannot reflect from host: %v", err),
			))

			continue
		}

		hosts = append(hosts, targetHost{
			Host: host,
			OS:   targets[i].OS.ValueString(),
			Arch: targets[i].Arch.ValueString(),
		})
	}

	return hosts, diags
}

func (r ResourceDeploymentStrategy) Equal(l ResourceDeploymentStrategy) bool {
	if !r.Type.Equal(l.Type) {
		return false
	}

	if r.Rolling != nil && l.Rolling != nil {
		return r.Rolling.Equal(*l.Rolling)
	} else if r.Rolling != nil || l.Rolling != nil {
		return false
	}

	return true
}

func (r ResourceDeploymentStrategyRolling) Equal(l ResourceDeploymentStrategyRolling) bool {
	return r.MaxSurge.Equal(l.MaxSurge)
}

func (r *ResourceDeployment) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = strings.Join([]string{req.ProviderTypeName, "deployment"}, "_")
}

func (r *ResourceDeployment) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `Specify how to deploy.`,
		Attributes: map[string]schema.Attribute{
			"artifact": schema.SingleNestedAttribute{
				Required:    true,
				Description: `The artifact of the deployment.`,
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Required:    true,
						Description: `The ID of the artifact.`,
					},
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
					"digest": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
						Description: `The digest of the artifact, in form of algorithm:checksum.`,
					},
				},
			},
			"targets": schema.ListNestedAttribute{
				Required:    true,
				Description: `The targets of the deployment.`,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Required:    true,
							Description: `The ID of the target.`,
						},
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
														Optional: true,
														Computed: true,
														Default:  stringdefault.StaticString("proxy"),
														Description: `The type to access the proxy, 
either "ssh" or "proxy".`,
														Validators: []validator.String{
															stringvalidator.OneOf("ssh", "proxy"),
														},
													},
													"user": schema.StringAttribute{
														Optional: true,
														Computed: true,
														Default:  stringdefault.StaticString(""),
														Description: `The user to authenticate 
when accessing the proxy.`,
													},
													"secret": schema.StringAttribute{
														Optional: true,
														Computed: true,
														Default:  stringdefault.StaticString(""),
														Description: `The secret to authenticate 
when accessing the proxy, either password or private key.`,
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
						"os": schema.StringAttribute{
							Required:    true,
							Description: `The operating system of the target.`,
						},
						"arch": schema.StringAttribute{
							Required:    true,
							Description: `The architecture of the target.`,
						},
					},
				},
			},
			"strategy": schema.SingleNestedAttribute{
				Optional:    true,
				Description: `Specify the strategy of the deployment.`,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Optional: true,
						Computed: true,
						Default:  stringdefault.StaticString("recreate"),
						Description: `The type of the deployment strategy,
either "recreate" or "rolling".`,
						Validators: []validator.String{
							stringvalidator.OneOf("recreate", "rolling"),
						},
					},
					"rolling": schema.SingleNestedAttribute{
						Optional:    true,
						Description: `The rolling strategy of the deployment.`,
						Attributes: map[string]schema.Attribute{
							"max_surge": schema.Float64Attribute{
								Optional:    true,
								Computed:    true,
								Default:     float64default.StaticFloat64(0.3),
								Description: `The maximum percent of targets to deploy at once.`,
								Validators: []validator.Float64{
									float64validator.AtLeast(0.1),
									float64validator.AtMost(1),
								},
							},
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
			"id": schema.StringAttribute{
				Computed:    true,
				Description: `The ID of the deployment.`,
			},
		},
	}
}

func (r *ResourceDeployment) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
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

	var plan ResourceDeployment

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan._ProviderConfig = r._ProviderConfig
	plan.ID = types.StringValue(plan.Hash())

	// Apply.
	resp.Diagnostics.Append(plan.Apply(ctx, nil)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ResourceDeployment) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}

func (r *ResourceDeployment) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
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

	var plan, state ResourceDeployment

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan._ProviderConfig = r._ProviderConfig
	plan.ID = types.StringValue(plan.Hash())

	if !plan.Equal(state) {
		planTargetsIndex := make(map[string]struct{}, len(plan.Targets))
		for i := range plan.Targets {
			planTargetsIndex[plan.Targets[i].ID.ValueString()] = struct{}{}
		}

		releaseTargets := make([]ResourceDeploymentTarget, 0, len(state.Targets))
		for i := range state.Targets {
			if _, ok := planTargetsIndex[state.Targets[i].ID.ValueString()]; ok {
				continue
			}
			releaseTargets = append(releaseTargets, state.Targets[i])
		}

		// Release.
		state.Targets = releaseTargets
		resp.Diagnostics.Append(state.Release(ctx)...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Apply.
		resp.Diagnostics.Append(plan.Apply(ctx, &state.Artifact)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ResourceDeployment) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if _, ok := ctx.Deadline(); !ok {
		timeout, diags := r.Timeouts.Delete(ctx, 30*time.Minute)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var state ResourceDeployment

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Release.
	resp.Diagnostics.Append(state.Release(ctx)...)
}

func (r *ResourceDeployment) Configure(
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
