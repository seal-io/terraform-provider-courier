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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/sync/errgroup"

	"github.com/seal-io/terraform-provider-courier/pkg/runtime"
	"github.com/seal-io/terraform-provider-courier/pkg/target"
	"github.com/seal-io/terraform-provider-courier/utils/osx"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

var _ resource.Resource = (*ResourceDeployment)(nil)

type (
	ResourceDeployment struct {
		Targets  []ResourceDeploymentTarget  `tfsdk:"targets"`
		Artifact ResourceDeploymentArtifact  `tfsdk:"artifact"`
		Runtime  ResourceDeploymentRuntime   `tfsdk:"runtime"`
		Strategy *ResourceDeploymentStrategy `tfsdk:"strategy"`
		Timeouts timeouts.Value              `tfsdk:"timeouts"`

		ID types.String `tfsdk:"id"`
	}

	ResourceDeploymentTarget struct {
		Host DataSourceTargetHost `tfsdk:"host"`
		OS   types.String         `tfsdk:"os"`
		Arch types.String         `tfsdk:"arch"`
	}

	ResourceDeploymentArtifact struct {
		Refer   DataSourceArtifactRefer `tfsdk:"refer"`
		Command types.String            `tfsdk:"command"`
		Ports   []types.Int64           `tfsdk:"ports"`
		Envs    map[string]types.String `tfsdk:"envs"`
		Volumes []types.String          `tfsdk:"volumes"`
		Digest  types.String            `tfsdk:"digest"`
	}

	ResourceDeploymentRuntime struct {
		Class    types.String            `tfsdk:"class"`
		Source   types.String            `tfsdk:"source"`
		Authn    *DataSourceRuntimeAuthn `tfsdk:"authn"`
		Insecure types.Bool              `tfsdk:"insecure"`
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

func (r *ResourceDeploymentArtifact) Equal(l ResourceDeploymentArtifact) bool {
	if r == nil {
		return false
	}

	return r.Refer.URI.Equal(l.Refer.URI)
}

func (r *ResourceDeployment) TargetsChanged(l ResourceDeployment) bool {
	if len(r.Targets) != len(l.Targets) {
		return true
	}

	rtg := append(make([]ResourceDeploymentTarget, 0, len(r.Targets)), r.Targets...)
	ltg := append(make([]ResourceDeploymentTarget, 0, len(l.Targets)), l.Targets...)

	sort.Slice(rtg, func(i, j int) bool {
		return rtg[i].Host.Address.ValueString() < rtg[j].Host.Address.ValueString()
	})
	sort.Slice(ltg, func(i, j int) bool {
		return ltg[i].Host.Address.ValueString() < ltg[j].Host.Address.ValueString()
	})

	for i := range ltg {
		if !rtg[i].Host.Address.Equal(ltg[i].Host.Address) {
			return true
		}
	}

	return false
}

func (r *ResourceDeployment) Apply(
	ctx context.Context,
	prevArt *ResourceDeploymentArtifact,
) diag.Diagnostics {
	deploy, diags := r.Reflect(ctx)
	if diags.HasError() {
		return diags
	}

	diags.Append(deploy.Setup(ctx)...)
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

		step := int(math.Round(maxSurge * float64(len(deploy.Targets))))
		if step == 0 {
			step = 1
		}

		if step != len(deploy.Targets) {
			for i, m := 0, len(deploy.Targets); i < m; {
				j := i + step
				if j > m {
					j = m
				}

				partialDeploy := *deploy
				if i == m-1 {
					partialDeploy.Targets = []DeploymentTarget{deploy.Targets[i]}
				} else {
					partialDeploy.Targets = append([]DeploymentTarget(nil), deploy.Targets[i:j]...)
				}

				if !prevArt.Equal(r.Artifact) {
					diags.Append(partialDeploy.Stop(ctx)...)
					if diags.HasError() {
						return diags
					}
				}

				diags.Append(partialDeploy.Start(ctx)...)
				if diags.HasError() {
					return diags
				}

				i = j
			}

			return diags
		}
	}

	// Recreate.
	if !prevArt.Equal(r.Artifact) {
		diags.Append(deploy.Stop(ctx)...)
		if diags.HasError() {
			return diags
		}
	}

	diags.Append(deploy.Start(ctx)...)

	return diags
}

func (r *ResourceDeployment) Release(
	ctx context.Context,
) diag.Diagnostics {
	deploy, diags := r.Reflect(ctx)
	if diags.HasError() {
		return diags
	}

	diags.Append(deploy.Cleanup(ctx)...)

	return diags
}

type (
	DeploymentTarget struct {
		target.Host

		RuntimeClass string
		OS           string
		Arch         string
	}

	Deployment struct {
		ID       string
		Targets  []DeploymentTarget
		Runtime  runtime.Source
		Artifact ResourceDeploymentArtifact
	}
)

func (r *ResourceDeployment) Reflect(
	ctx context.Context,
) (*Deployment, diag.Diagnostics) {
	var diags diag.Diagnostics

	rt, err := r.Runtime.Reflect(ctx)
	if err != nil {
		diags.Append(diag.NewAttributeErrorDiagnostic(
			path.Root("runtime"),
			"Invalid Runtime",
			fmt.Sprintf("Cannot reflect: %v", err),
		))

		return nil, diags
	}

	deploy := &Deployment{
		ID:       r.ID.ValueString(),
		Targets:  make([]DeploymentTarget, 0, len(r.Targets)),
		Runtime:  rt,
		Artifact: r.Artifact,
	}

	for i := range r.Targets {
		host, err := r.Targets[i].Host.Reflect(ctx)
		if err != nil {
			diags.Append(diag.NewAttributeErrorDiagnostic(
				path.Root("targets").AtListIndex(i).AtName("host"),
				"Invalid Target Host",
				fmt.Sprintf("Cannot reflect from host: %v", err),
			))

			continue
		}

		deploy.Targets = append(deploy.Targets, DeploymentTarget{
			Host:         host,
			RuntimeClass: r.Runtime.Class.ValueString(),
			OS:           r.Targets[i].OS.ValueString(),
			Arch:         r.Targets[i].Arch.ValueString(),
		})
	}

	return deploy, diags
}

func (t DeploymentTarget) Command() string {
	suffix := "sh"
	if t.OS == "windows" {
		suffix = "ps1"
	}

	return fmt.Sprintf("/var/local/courier/runtime/%s/%s/service.%s",
		t.RuntimeClass,
		t.OS,
		suffix)
}

func (d Deployment) Setup(ctx context.Context) diag.Diagnostics {
	var (
		diags diag.Diagnostics
		tgts  = d.Targets
		art   = d.Artifact
	)

	// Upload runtime.
	{
		g, ctx := errgroup.WithContext(ctx)

		for i := range tgts {
			t := tgts[i]

			g.Go(func() error {
				// TODO, avoid upload runtime if already exists.
				err := t.UploadDirectory(
					ctx,
					d.Runtime,
					"/var/local/courier/runtime")
				if err != nil {
					return err
				}

				if t.OS == "linux" {
					output, err := t.ExecuteWithOutput(
						ctx,
						"chmod",
						"a+x",
						fmt.Sprintf("/var/local/courier/runtime/%s/linux/service.sh", t.RuntimeClass),
					)
					if err != nil {
						tflog.Error(ctx, "cannot change service permission: "+string(output))
						return err
					}
				}

				return nil
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

	// Upload artifact.
	{
		tmpDir := osx.TempDir("courier-")
		defer func() { _ = os.RemoveAll(tmpDir) }()

		err := os.WriteFile( //nolint:gosec
			fmt.Sprintf("%s/command", tmpDir),
			[]byte(art.Command.ValueString()),
			0o666,
		)
		if err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot prepare command",
				fmt.Sprintf("Cannot write command: %v", err),
			))

			return diags
		}

		var (
			ports    = make([]int, 0, len(art.Ports))
			portsBuf bytes.Buffer
		)
		for i := range art.Ports {
			if art.Ports[i].IsNull() ||
				art.Ports[i].IsUnknown() {
				continue
			}
			ports = append(ports, int(art.Ports[i].ValueInt64()))
		}
		sort.Ints(ports)
		for i := range ports {
			_, _ = fmt.Fprintf(&portsBuf, "%d\n", ports[i])
		}
		err = os.WriteFile( //nolint:gosec
			fmt.Sprintf("%s/ports", tmpDir),
			portsBuf.Bytes(),
			0o666)
		if err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot prepare ports",
				fmt.Sprintf("Cannot prepare ports: %v", err),
			))

			return diags
		}

		var (
			envs    = make([]string, 0, len(art.Envs))
			envsBuf bytes.Buffer
		)
		for k, v := range art.Envs {
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
		err = os.WriteFile( //nolint:gosec
			fmt.Sprintf("%s/envs", tmpDir),
			envsBuf.Bytes(),
			0o666)
		if err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot prepare envs",
				fmt.Sprintf("Cannot prepare envs: %v", err),
			))

			return diags
		}

		var (
			volumes    = make([]string, 0, len(art.Volumes))
			volumesBuf bytes.Buffer
		)
		for i := range art.Volumes {
			if art.Volumes[i].IsNull() ||
				art.Volumes[i].IsUnknown() {
				continue
			}
			volumes = append(volumes, art.Volumes[i].ValueString())
		}
		sort.Strings(volumes)
		for i := range volumes {
			_, _ = fmt.Fprintf(&volumesBuf, "%s\n", volumes[i])
		}
		err = os.WriteFile( //nolint:gosec
			fmt.Sprintf("%s/volumes", tmpDir),
			volumesBuf.Bytes(),
			0o666,
		)
		if err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot prepare volumes",
				fmt.Sprintf("Cannot prepare volumes: %v", err),
			))

			return diags
		}

		g, ctx := errgroup.WithContext(ctx)

		for i := range tgts {
			t := tgts[i]

			g.Go(func() (err error) {
				return t.UploadDirectory(
					ctx,
					os.DirFS(tmpDir),
					fmt.Sprintf("/var/local/courier/artifact/%s", d.ID))
			})
		}

		if err := g.Wait(); err != nil {
			diags.Append(diag.NewErrorDiagnostic(
				"Cannot upload artifact",
				fmt.Sprintf("Cannot upload artifact: %v", err),
			))

			return diags
		}
	}

	{
		args := []string{
			"setup",
			d.ID,
			art.Refer.URI.ValueString(),
			art.Digest.ValueString(),
		}

		if au := art.Refer.Authn; au != nil {
			args = append(
				args,
				au.Type.ValueString(),
				au.User.ValueString(),
				au.Secret.ValueString(),
			)
		}

		diags.Append(d.execute(ctx, args...)...)
	}

	return diags
}

func (d Deployment) Start(ctx context.Context) diag.Diagnostics {
	return d.execute(ctx, "start", d.ID)
}

func (d Deployment) Stop(ctx context.Context) diag.Diagnostics {
	return d.execute(ctx, "stop", d.ID)
}

func (d Deployment) Cleanup(ctx context.Context) diag.Diagnostics {
	return d.execute(ctx, "cleanup", d.ID)
}

func (d Deployment) execute(ctx context.Context, args ...string) diag.Diagnostics {
	if len(args) == 0 {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Cannot execute",
				"Cannot execute without arguments",
			),
		}
	}

	g, ctx := errgroup.WithContext(ctx)

	for i := range d.Targets {
		t := d.Targets[i]

		g.Go(func() error {
			output, err := t.ExecuteWithOutput(
				ctx,
				t.Command(),
				args...,
			)
			if err != nil {
				tflog.Error(ctx, "cannot execute "+args[0]+": "+string(output))
			}

			return err
		})
	}

	if err := g.Wait(); err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Cannot execute "+args[0],
				fmt.Sprintf("Cannot execute %s: %v", args[0], err),
			),
		}
	}

	return nil
}

func (r ResourceDeploymentRuntime) Reflect(
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

func (r ResourceDeploymentStrategy) Equal(
	l ResourceDeploymentStrategy,
) bool {
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

func (r ResourceDeploymentStrategyRolling) Equal(
	l ResourceDeploymentStrategyRolling,
) bool {
	return r.MaxSurge.Equal(l.MaxSurge)
}

func (r *ResourceDeployment) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = strings.Join(
		[]string{req.ProviderTypeName, "deployment"},
		"_",
	)
}

func (r *ResourceDeployment) Schema(
	ctx context.Context,
	req resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: `Specify how to deploy.`,
		Attributes: map[string]schema.Attribute{
			"targets": schema.ListNestedAttribute{
				Required:    true,
				Description: `The targets of the deployment.`,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
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
											Optional: true,
											Computed: true,
											Default: stringdefault.StaticString(
												"ssh",
											),
											Description: `The type to access the target, either "ssh" or "winrm".`,
											Validators: []validator.String{
												stringvalidator.OneOf(
													"ssh",
													"winrm",
												),
											},
										},
										"user": schema.StringAttribute{
											Optional: true,
											Computed: true,
											Default: stringdefault.StaticString(
												"root",
											),
											Description: `The user to authenticate when accessing the target.`,
										},
										"secret": schema.StringAttribute{
											Optional: true,
											Computed: true,
											Default: stringdefault.StaticString(
												"",
											),
											Description: `The secret to authenticate when accessing the target, 
either password or private key.`,
											Sensitive: true,
										},
										"agent": schema.BoolAttribute{
											Optional: true,
											Computed: true,
											Default: booldefault.StaticBool(
												false,
											),
											Description: `Specify to access the target with agent,
either SSH agent if type is "ssh" or NTLM if type is "winrm".`,
										},
									},
								},
								"insecure": schema.BoolAttribute{
									Optional: true,
									Computed: true,
									Default: booldefault.StaticBool(
										false,
									),
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
													stringvalidator.LengthAtLeast(
														1,
													),
												},
											},
											"authn": schema.SingleNestedAttribute{
												Required:    true,
												Description: `The authentication for accessing the proxy.`,
												Attributes: map[string]schema.Attribute{
													"type": schema.StringAttribute{
														Optional: true,
														Computed: true,
														Default: stringdefault.StaticString(
															"proxy",
														),
														Description: `The type to access the proxy, 
either "ssh" or "proxy".`,
														Validators: []validator.String{
															stringvalidator.OneOf(
																"ssh",
																"proxy",
															),
														},
													},
													"user": schema.StringAttribute{
														Optional: true,
														Computed: true,
														Default: stringdefault.StaticString(
															"",
														),
														Description: `The user to authenticate 
when accessing the proxy.`,
													},
													"secret": schema.StringAttribute{
														Optional: true,
														Computed: true,
														Default: stringdefault.StaticString(
															"",
														),
														Description: `The secret to authenticate 
when accessing the proxy, either password or private key.`,
														Sensitive: true,
													},
												},
											},
											"insecure": schema.BoolAttribute{
												Optional: true,
												Computed: true,
												Default: booldefault.StaticBool(
													false,
												),
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
			"runtime": schema.SingleNestedAttribute{
				Required:    true,
				Description: `The runtime of the deployment.`,
				Attributes: map[string]schema.Attribute{
					"class": schema.StringAttribute{
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
						Required:    true,
						Description: `Specify the class of the runtime.`,
					},
					"source": schema.StringAttribute{
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
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
						Description: `The authentication for fetching the runtime.`,
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
				},
			},
			"artifact": schema.SingleNestedAttribute{
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Required:    true,
				Description: `The artifact of the deployment.`,
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
										Optional: true,
										Computed: true,
										Default: stringdefault.StaticString(
											"basic",
										),
										Description: `The type for authentication, either "basic" or "bearer".`,
										Validators: []validator.String{
											stringvalidator.OneOf(
												"basic",
												"bearer",
											),
										},
									},
									"user": schema.StringAttribute{
										Optional: true,
										Computed: true,
										Default: stringdefault.StaticString(
											"",
										),
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
					"command": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
						Description: `The command to start the artifact.`,
					},
					"ports": schema.ListAttribute{
						Optional: true,
						Computed: true,
						Default: listdefault.StaticValue(
							basetypes.NewListNull(types.Int64Type),
						),
						Description: `The ports of the artifact.`,
						ElementType: types.Int64Type,
					},
					"envs": schema.MapAttribute{
						Optional: true,
						Computed: true,
						Default: mapdefault.StaticValue(
							basetypes.NewMapNull(types.StringType),
						),
						Description: `The environment variables of the artifact.`,
						ElementType: types.StringType,
					},
					"volumes": schema.ListAttribute{
						Optional: true,
						Computed: true,
						Default: listdefault.StaticValue(
							basetypes.NewListNull(types.StringType),
						),
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
								Optional: true,
								Computed: true,
								Default: float64default.StaticFloat64(
									0.3,
								),
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

func (r *ResourceDeployment) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan ResourceDeployment

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = types.StringValue(strx.Sum(
		plan.Artifact.Refer.URI.ValueString(),
		strx.Hex(64)))

	{
		// Get timeout.
		timeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Apply.
		resp.Diagnostics.Append(plan.Apply(ctx, nil)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ResourceDeployment) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
}

func (r *ResourceDeployment) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan, state ResourceDeployment

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.TargetsChanged(state) {
		tflog.Debug(ctx, "Targets changed, applying again...")

		plan.ID = state.ID

		// Diff.
		planTargetsIndex := make(map[string]struct{}, len(plan.Targets))
		for i := range plan.Targets {
			planTargetsIndex[plan.Targets[i].Host.Address.ValueString()] = struct{}{}
		}

		releaseTargets := make([]ResourceDeploymentTarget, 0, len(state.Targets))
		for i := range state.Targets {
			if _, ok := planTargetsIndex[state.Targets[i].Host.Address.ValueString()]; ok {
				continue
			}
			releaseTargets = append(releaseTargets, state.Targets[i])
		}

		// Get Timeout.
		timeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

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

func (r *ResourceDeployment) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state ResourceDeployment

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get Timeout.
	timeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Release.
	resp.Diagnostics.Append(state.Release(ctx)...)
	if resp.Diagnostics.HasError() {
		return
	}
}
