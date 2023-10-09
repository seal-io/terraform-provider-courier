package courier

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"

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
	Provider struct{}
)

func NewProvider() provider.Provider {
	return &Provider{}
}

func (p *Provider) Metadata(
	ctx context.Context,
	req provider.MetadataRequest,
	resp *provider.MetadataResponse,
) {
	resp.TypeName = ProviderType
	resp.Version = version.Version
}

func (p *Provider) Schema(
	ctx context.Context,
	req provider.SchemaRequest,
	resp *provider.SchemaResponse,
) {
}

func (p *Provider) Configure(
	ctx context.Context,
	req provider.ConfigureRequest,
	resp *provider.ConfigureResponse,
) {
}

func (p *Provider) DataSources(
	ctx context.Context,
) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDataSourceArtifact,
		NewDataSourceRuntime,
		NewDataSourceTarget,
	}
}

func (p *Provider) Resources(
	ctx context.Context,
) []func() resource.Resource {
	return []func() resource.Resource{
		NewResourceDeployment,
	}
}
