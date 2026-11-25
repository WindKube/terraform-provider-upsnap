package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var (
	_ provider.Provider            = &UpSnapProvider{}
	_ provider.ProviderWithActions = &UpSnapProvider{}
)

type UpSnapProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &UpSnapProvider{version: version}
	}
}

func (p *UpSnapProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "upsnap"
	resp.Version = p.version
}

func (p *UpSnapProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage [UpSnap](https://github.com/seriousm4x/UpSnap) wake-on-LAN dashboards.",
	}
}

func (p *UpSnapProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

func (p *UpSnapProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *UpSnapProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *UpSnapProvider) Actions(_ context.Context) []func() action.Action {
	return nil
}
