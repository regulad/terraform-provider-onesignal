// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal"
)

var (
	_ datasource.DataSource = &AppDataSource{}
	_ datasource.DataSource = &AppsDataSource{}
)

// NewAppDataSource returns the onesignal_app data source.
func NewAppDataSource() datasource.DataSource { return &AppDataSource{} }

// NewAppsDataSource returns the onesignal_apps data source.
func NewAppsDataSource() datasource.DataSource { return &AppsDataSource{} }

// AppDataSource reads one app.
type AppDataSource struct {
	client *onesignal.Client
}

type appDataModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	OrganizationID   types.String `tfsdk:"organization_id"`
	ChromeWebOrigin  types.String `tfsdk:"chrome_web_origin"`
	SiteName         types.String `tfsdk:"site_name"`
	SafariSiteOrigin types.String `tfsdk:"safari_site_origin"`
	APNSEnv          types.String `tfsdk:"apns_env"`
	APNSBundleID     types.String `tfsdk:"apns_bundle_id"`
	APNSConfigured   types.Bool   `tfsdk:"apns_configured"`
	FCMConfigured    types.Bool   `tfsdk:"fcm_configured"`
	WebConfigured    types.Bool   `tfsdk:"web_configured"`
}

func (d *AppDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (d *AppDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{MarkdownDescription: desc, Computed: true}
	}
	flag := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{MarkdownDescription: desc, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An app of the Organization API key's organization, by its App ID.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{MarkdownDescription: "The App ID.", Required: true},
			"name":               computed("The app's name."),
			"organization_id":    computed("The app's organization."),
			"chrome_web_origin":  computed("The web push origin."),
			"site_name":          computed("The website's name."),
			"safari_site_origin": computed("The Safari web push origin."),
			"apns_env":           computed("The APNs environment."),
			"apns_bundle_id":     computed("The iOS app's bundle ID."),
			"apns_configured":    flag("Whether OneSignal holds an APNs credential."),
			"fcm_configured":     flag("Whether OneSignal holds a Firebase credential."),
			"web_configured":     flag("Whether web push is configured."),
		},
	}
}

func (d *AppDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *AppDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m appDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := d.client.GetApp(ctx, m.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the app", err.Error())
		return
	}
	m.Name = types.StringValue(a.Name)
	m.OrganizationID = types.StringValue(a.OrganizationID)
	m.ChromeWebOrigin = types.StringValue(a.ChromeWebOrigin)
	m.SiteName = types.StringValue(a.SiteName)
	m.SafariSiteOrigin = types.StringValue(a.SafariSiteOrigin)
	m.APNSEnv = types.StringValue(a.APNSEnv)
	m.APNSBundleID = types.StringValue(a.APNSBundleID)
	m.APNSConfigured = types.BoolValue(a.APNSEnv != "")
	m.FCMConfigured = types.BoolValue(a.FCMSenderID != "")
	m.WebConfigured = types.BoolValue(a.ChromeWebOrigin != "")
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// AppsDataSource lists the organization's apps.
type AppsDataSource struct {
	client *onesignal.Client
}

type appsDataModel struct {
	Apps []appSummary `tfsdk:"apps"`
}

type appSummary struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *AppsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apps"
}

func (d *AppsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Every app of the Organization API key's organization, sorted by name.",
		Attributes: map[string]schema.Attribute{
			"apps": schema.ListNestedAttribute{
				MarkdownDescription: "The apps.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{MarkdownDescription: "The App ID.", Computed: true},
						"name": schema.StringAttribute{MarkdownDescription: "The app's name.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *AppsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *AppsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apps, err := d.client.ListApps(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing the apps", err.Error())
		return
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	m := appsDataModel{Apps: []appSummary{}}
	for _, a := range apps {
		m.Apps = append(m.Apps, appSummary{ID: types.StringValue(a.ID), Name: types.StringValue(a.Name)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
