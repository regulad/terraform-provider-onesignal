// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal"
)

var (
	_ resource.Resource                = &AppResource{}
	_ resource.ResourceWithImportState = &AppResource{}
)

// NewAppResource returns the onesignal_app resource.
func NewAppResource() resource.Resource { return &AppResource{} }

// AppResource is a OneSignal app.
type AppResource struct {
	client *onesignal.Client
}

type appModel struct {
	ID                               types.String `tfsdk:"id"`
	Name                             types.String `tfsdk:"name"`
	OrganizationID                   types.String `tfsdk:"organization_id"`
	ChromeWebOrigin                  types.String `tfsdk:"chrome_web_origin"`
	SiteName                         types.String `tfsdk:"site_name"`
	SafariSiteOrigin                 types.String `tfsdk:"safari_site_origin"`
	ChromeWebDefaultNotificationIcon types.String `tfsdk:"chrome_web_default_notification_icon"`
	SafariIcon256                    types.String `tfsdk:"safari_icon_256_256"`
	APNSEnv                          types.String `tfsdk:"apns_env"`
	APNSKeyID                        types.String `tfsdk:"apns_key_id"`
	APNSTeamID                       types.String `tfsdk:"apns_team_id"`
	APNSBundleID                     types.String `tfsdk:"apns_bundle_id"`
	AdditionalDataIsRootPayload      types.Bool   `tfsdk:"additional_data_is_root_payload"`

	APNSP8WO                types.String `tfsdk:"apns_p8_wo"`
	APNSP8WOVersion         types.Int64  `tfsdk:"apns_p8_wo_version"`
	APNSP12WO               types.String `tfsdk:"apns_p12_wo"`
	APNSP12PasswordWO       types.String `tfsdk:"apns_p12_password_wo"`
	APNSP12WOVersion        types.Int64  `tfsdk:"apns_p12_wo_version"`
	FCMServiceAccountWO     types.String `tfsdk:"fcm_v1_service_account_json_wo"`
	FCMServiceAccountWOVer  types.Int64  `tfsdk:"fcm_v1_service_account_json_wo_version"`
	SafariAPNSP12WO         types.String `tfsdk:"safari_apns_p12_wo"`
	SafariAPNSP12PasswordWO types.String `tfsdk:"safari_apns_p12_password_wo"`
	SafariAPNSP12WOVersion  types.Int64  `tfsdk:"safari_apns_p12_wo_version"`
	APNSConfigured          types.Bool   `tfsdk:"apns_configured"`
	FCMConfigured           types.Bool   `tfsdk:"fcm_configured"`
	WebConfigured           types.Bool   `tfsdk:"web_configured"`
	ChromeWebKey            types.String `tfsdk:"chrome_web_key"`
	SafariPushID            types.String `tfsdk:"safari_push_id"`
	FCMSenderID             types.String `tfsdk:"fcm_sender_id"`
}

// appSetting ties a plain setting to its API field and its value as read.
type appSetting struct {
	api  string
	attr func(*appModel) *types.String
	read func(*onesignal.App) string
}

var appSettings = []appSetting{
	{"chrome_web_origin", func(m *appModel) *types.String { return &m.ChromeWebOrigin }, func(a *onesignal.App) string { return a.ChromeWebOrigin }},
	{"safari_site_origin", func(m *appModel) *types.String { return &m.SafariSiteOrigin }, func(a *onesignal.App) string { return a.SafariSiteOrigin }},
	{"safari_icon_256_256", func(m *appModel) *types.String { return &m.SafariIcon256 }, func(a *onesignal.App) string { return a.SafariIcon256 }},
	{"apns_env", func(m *appModel) *types.String { return &m.APNSEnv }, func(a *onesignal.App) string { return a.APNSEnv }},
	{"apns_key_id", func(m *appModel) *types.String { return &m.APNSKeyID }, func(a *onesignal.App) string { return a.APNSKeyID }},
	{"apns_team_id", func(m *appModel) *types.String { return &m.APNSTeamID }, func(a *onesignal.App) string { return a.APNSTeamID }},
	{"apns_bundle_id", func(m *appModel) *types.String { return &m.APNSBundleID }, func(a *onesignal.App) string { return a.APNSBundleID }},
}

// appCredential is a write-only credential set, resent when its version moves.
type appCredential struct {
	version func(*appModel) types.Int64
	fields  map[string]func(*appModel) types.String
}

var appCredentials = []appCredential{
	{func(m *appModel) types.Int64 { return m.APNSP8WOVersion }, map[string]func(*appModel) types.String{
		"apns_p8": func(m *appModel) types.String { return m.APNSP8WO },
	}},
	{func(m *appModel) types.Int64 { return m.APNSP12WOVersion }, map[string]func(*appModel) types.String{
		"apns_p12":          func(m *appModel) types.String { return m.APNSP12WO },
		"apns_p12_password": func(m *appModel) types.String { return m.APNSP12PasswordWO },
	}},
	{func(m *appModel) types.Int64 { return m.FCMServiceAccountWOVer }, map[string]func(*appModel) types.String{
		"fcm_v1_service_account_json": func(m *appModel) types.String { return m.FCMServiceAccountWO },
	}},
	{func(m *appModel) types.Int64 { return m.SafariAPNSP12WOVersion }, map[string]func(*appModel) types.String{
		"safari_apns_p12":          func(m *appModel) types.String { return m.SafariAPNSP12WO },
		"safari_apns_p12_password": func(m *appModel) types.String { return m.SafariAPNSP12PasswordWO },
	}},
}

func (r *AppResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func optionalComputedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc + " Left unset, it is neither sent nor compared.",
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func readOnlyString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func writeOnly(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc + " Write-only: never stored or compared; sent on create and when its version changes.",
		Optional:            true,
		Sensitive:           true,
		WriteOnly:           true,
	}
}

func woVersion(what string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: "Change it to send " + what + " again.",
		Optional:            true,
	}
}

func (r *AppResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A OneSignal app. OneSignal has no API to delete an app, so destroying one fails: delete it in the dashboard, then remove it from state. " +
			"Push credentials are write-only, as base64 the way OneSignal takes them (`filebase64()`), and resent when their `*_wo_version` changes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The App ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The app's name.",
				Required:            true,
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The organization the app belongs to. Changing it would replace the app, which OneSignal cannot delete.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"chrome_web_origin":                    optionalComputedString("The HTTPS origin of the website, for web push."),
			"site_name":                            readOnlyString("The website's name, used in web push titles. OneSignal's API accepts it and does not change it, so it is set in the dashboard."),
			"safari_site_origin":                   optionalComputedString("The HTTPS origin for Safari web push."),
			"chrome_web_default_notification_icon": readOnlyString("An HTTPS URL of the default web push icon. OneSignal's API accepts it and does not change it, so it is set in the dashboard."),
			"safari_icon_256_256":                  optionalComputedString("A 256×256 icon for legacy Safari."),
			"apns_env":                             optionalComputedString("The APNs environment of the credential. OneSignal keeps it, like the other APNs settings, only alongside an APNs credential, sent with it or already held."),
			"apns_key_id":                          optionalComputedString("The APNs Key ID of the `.p8` key."),
			"apns_team_id":                         optionalComputedString("The Apple Team ID."),
			"apns_bundle_id":                       optionalComputedString("The iOS app's bundle ID."),
			"additional_data_is_root_payload": schema.BoolAttribute{
				MarkdownDescription: "Whether a notification's `data` goes in the push payload's root. OneSignal's API sets it to true and does not set it back, so it is set in the dashboard.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"apns_p8_wo":                             writeOnly("The APNs `.p8` key, base64."),
			"apns_p8_wo_version":                     woVersion("`apns_p8_wo`"),
			"apns_p12_wo":                            writeOnly("The APNs `.p12` certificate, base64."),
			"apns_p12_password_wo":                   writeOnly("The APNs `.p12` certificate's password."),
			"apns_p12_wo_version":                    woVersion("`apns_p12_wo` and its password"),
			"fcm_v1_service_account_json_wo":         writeOnly("The Firebase service account's JSON, base64."),
			"fcm_v1_service_account_json_wo_version": woVersion("`fcm_v1_service_account_json_wo`"),
			"safari_apns_p12_wo":                     writeOnly("The Safari web push `.p12` certificate, base64."),
			"safari_apns_p12_password_wo":            writeOnly("The Safari `.p12` certificate's password."),
			"safari_apns_p12_wo_version":             woVersion("`safari_apns_p12_wo` and its password"),
			"apns_configured": schema.BoolAttribute{
				MarkdownDescription: "Whether OneSignal holds an APNs credential.",
				Computed:            true,
			},
			"fcm_configured": schema.BoolAttribute{
				MarkdownDescription: "Whether OneSignal holds a Firebase credential.",
				Computed:            true,
			},
			"web_configured": schema.BoolAttribute{
				MarkdownDescription: "Whether web push is configured.",
				Computed:            true,
			},
			"chrome_web_key": schema.StringAttribute{
				MarkdownDescription: "The web push VAPID public key.",
				Computed:            true,
			},
			"safari_push_id": schema.StringAttribute{
				MarkdownDescription: "The Safari web push ID.",
				Computed:            true,
			},
			"fcm_sender_id": schema.StringAttribute{
				MarkdownDescription: "The Firebase sender ID.",
				Computed:            true,
			},
		},
	}
}

func (r *AppResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// setFromApp copies what OneSignal returns into m.
func setFromApp(m *appModel, a *onesignal.App) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.OrganizationID = types.StringValue(a.OrganizationID)
	for _, s := range appSettings {
		*s.attr(m) = types.StringValue(s.read(a))
	}
	m.SiteName = types.StringValue(a.SiteName)
	m.ChromeWebDefaultNotificationIcon = types.StringValue(a.ChromeWebDefaultNotificationIcon)
	m.AdditionalDataIsRootPayload = types.BoolValue(a.AdditionalDataIsRootPayload != nil && *a.AdditionalDataIsRootPayload)
	m.APNSConfigured = types.BoolValue(a.APNSEnv != "")
	m.FCMConfigured = types.BoolValue(a.FCMSenderID != "")
	m.WebConfigured = types.BoolValue(a.ChromeWebOrigin != "")
	m.ChromeWebKey = types.StringValue(a.ChromeWebKey)
	m.SafariPushID = types.StringValue(a.SafariPushID)
	m.FCMSenderID = types.StringValue(a.FCMSenderID)
}

// credentialFields adds the write-only values of each credential set for which
// send says so, read from config.
func credentialFields(fields onesignal.AppFields, config *appModel, send func(appCredential) bool) {
	for _, c := range appCredentials {
		if !send(c) {
			continue
		}
		for api, get := range c.fields {
			if v := get(config); !v.IsNull() && !v.IsUnknown() {
				fields[api] = v.ValueString()
			}
		}
	}
}

func (r *AppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config appModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fields := onesignal.AppFields{"name": plan.Name.ValueString(), "organization_id": plan.OrganizationID.ValueString()}
	for _, s := range appSettings {
		if v := *s.attr(&plan); !v.IsNull() && !v.IsUnknown() {
			fields[s.api] = v.ValueString()
		}
	}
	credentialFields(fields, &config, func(appCredential) bool { return true })
	id, err := r.client.CreateApp(ctx, fields)
	if err != nil {
		resp.Diagnostics.AddError("Creating the app", err.Error())
		return
	}
	app, err := r.client.GetApp(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading the new app", err.Error())
		return
	}
	setFromApp(&plan, app)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state appModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	app, err := r.client.GetApp(ctx, state.ID.ValueString())
	if onesignal.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading the app", err.Error())
		return
	}
	setFromApp(&state, app)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config appModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Only what changed is sent, so nothing else on the app is touched.
	fields := onesignal.AppFields{}
	if !plan.Name.Equal(state.Name) {
		fields["name"] = plan.Name.ValueString()
	}
	for _, s := range appSettings {
		if v := *s.attr(&plan); !v.IsNull() && !v.IsUnknown() && !v.Equal(*s.attr(&state)) {
			fields[s.api] = v.ValueString()
		}
	}
	credentialFields(fields, &config, func(c appCredential) bool { return !c.version(&plan).Equal(c.version(&state)) })
	if len(fields) > 0 {
		if err := r.client.UpdateApp(ctx, state.ID.ValueString(), fields); err != nil {
			resp.Diagnostics.AddError("Updating the app", err.Error())
			return
		}
	}
	app, err := r.client.GetApp(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the updated app", err.Error())
		return
	}
	setFromApp(&plan, app)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppResource) Delete(_ context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(context.Background(), path.Root("id"), &id)...)
	resp.Diagnostics.AddError("OneSignal cannot delete apps through its API",
		"Delete app "+id.ValueString()+" in OneSignal's dashboard (All Apps → Options → Delete), then remove it from state with a `removed` block or `tofu state rm`.")
}

func (r *AppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
