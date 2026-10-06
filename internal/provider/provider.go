package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &openaiProvider{}

type openaiProvider struct {
	version string
}

type openaiProviderModel struct {
	AdminAPIKey types.String `tfsdk:"admin_api_key"`
	BaseURL     types.String `tfsdk:"base_url"`
}

func (p *openaiProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "openai"
	resp.Version = p.version
}

func (p *openaiProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manage an OpenAI organization with Terraform through the Admin API.

This provider is not affiliated with or endorsed by OpenAI.

## Authentication

The provider sends an Admin API key as a bearer token on every request. Set
` + "`admin_api_key`" + `, or leave it out and set ` + "`OPENAI_ADMIN_KEY`" + `. Configuring
neither fails before any request is made.`,
		Attributes: map[string]schema.Attribute{
			"admin_api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "An OpenAI Admin API key. Falls back to `OPENAI_ADMIN_KEY`.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The OpenAI API base URL. Falls back to `OPENAI_BASE_URL`, then `" + defaultBaseURL + "`.",
			},
		},
	}
}

func (p *openaiProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data openaiProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.AdminAPIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("admin_api_key"), "Unknown OpenAI Admin API Key",
			"The admin_api_key value is not known until apply, so the provider cannot authenticate. Set it to a value known at plan time, or use OPENAI_ADMIN_KEY.")
	}
	if data.BaseURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("base_url"), "Unknown OpenAI Base URL",
			"The base_url value is not known until apply. Set it to a value known at plan time, or use OPENAI_BASE_URL.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := newClient(clientConfig{
		adminAPIKey: data.AdminAPIKey.ValueString(),
		baseURL:     data.BaseURL.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Missing OpenAI Admin API Key", err.Error())
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *openaiProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newProject,
		newProjectServiceAccount,
	}
}

func (p *openaiProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &openaiProvider{version: version}
	}
}
