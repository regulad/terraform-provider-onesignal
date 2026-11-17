// SPDX-License-Identifier: MPL-2.0

// Package provider is the OneSignal provider: apps, app API keys, segments and
// templates.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal"
)

// Environment variables the provider falls back to.
const (
	EnvOrganizationAPIKey = "ONESIGNAL_ORGANIZATION_API_KEY"
	EnvBaseURL            = "ONESIGNAL_BASE_URL"
)

var _ provider.Provider = &OneSignalProvider{}

// OneSignalProvider is the provider.
type OneSignalProvider struct {
	version string
}

type providerModel struct {
	OrganizationAPIKey types.String `tfsdk:"organization_api_key"`
	BaseURL            types.String `tfsdk:"base_url"`
}

// New returns the provider for version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &OneSignalProvider{version: version}
	}
}

func (p *OneSignalProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "onesignal"
	resp.Version = p.version
}

func (p *OneSignalProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages OneSignal's configuration: apps, app API keys, segments and templates.",
		Attributes: map[string]schema.Attribute{
			"organization_api_key": schema.StringAttribute{
				MarkdownDescription: "An Organization API key, made in the OneSignal dashboard's organization settings. It manages apps and their API keys, and segments and templates unless they are given an app's own key. Falls back to `" + EnvOrganizationAPIKey + "`.",
				Optional:            true,
				Sensitive:           true,
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "OneSignal's API. Defaults to `" + onesignal.DefaultBaseURL + "`, or `" + EnvBaseURL + "`.",
				Optional:            true,
			},
		},
	}
}

func (p *OneSignalProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key := config.OrganizationAPIKey.ValueString()
	if key == "" {
		key = os.Getenv(EnvOrganizationAPIKey)
	}
	baseURL := config.BaseURL.ValueString()
	if baseURL == "" {
		baseURL = os.Getenv(EnvBaseURL)
	}
	if key == "" && !config.OrganizationAPIKey.IsUnknown() {
		resp.Diagnostics.AddError("Missing Organization API key",
			"Set organization_api_key or "+EnvOrganizationAPIKey+" to a OneSignal Organization API key, made in the dashboard's organization settings.")
		return
	}
	client := onesignal.New(baseURL, key, "terraform-provider-onesignal/"+p.version)
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *OneSignalProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAppResource,
		NewAppAPIKeyResource,
		NewSegmentResource,
		NewTemplateResource,
	}
}

func (p *OneSignalProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAppDataSource,
		NewAppsDataSource,
	}
}
