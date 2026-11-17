// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal"
)

var (
	_ resource.Resource                   = &TemplateResource{}
	_ resource.ResourceWithImportState    = &TemplateResource{}
	_ resource.ResourceWithValidateConfig = &TemplateResource{}
)

// NewTemplateResource returns the onesignal_template resource.
func NewTemplateResource() resource.Resource { return &TemplateResource{} }

// TemplateResource is a message template of an app.
type TemplateResource struct {
	client *onesignal.Client
}

type templateModel struct {
	ID                        types.String `tfsdk:"id"`
	AppID                     types.String `tfsdk:"app_id"`
	APIKey                    types.String `tfsdk:"api_key"`
	Name                      types.String `tfsdk:"name"`
	Channel                   types.String `tfsdk:"channel"`
	Contents                  types.Map    `tfsdk:"contents"`
	Headings                  types.Map    `tfsdk:"headings"`
	Subtitle                  types.Map    `tfsdk:"subtitle"`
	URL                       types.String `tfsdk:"url"`
	AppURL                    types.String `tfsdk:"app_url"`
	WebURL                    types.String `tfsdk:"web_url"`
	BigPicture                types.String `tfsdk:"big_picture"`
	ChromeWebImage            types.String `tfsdk:"chrome_web_image"`
	Data                      types.Map    `tfsdk:"data"`
	PushPlatforms             types.Set    `tfsdk:"push_platforms"`
	EmailSubject              types.String `tfsdk:"email_subject"`
	EmailBody                 types.String `tfsdk:"email_body"`
	EmailPreheader            types.String `tfsdk:"email_preheader"`
	EmailFromName             types.String `tfsdk:"email_from_name"`
	EmailFromAddress          types.String `tfsdk:"email_from_address"`
	EmailReplyToAddress       types.String `tfsdk:"email_reply_to_address"`
	EmailBCC                  types.List   `tfsdk:"email_bcc"`
	DisableEmailClickTracking types.Bool   `tfsdk:"disable_email_click_tracking"`
	SMSFrom                   types.String `tfsdk:"sms_from"`
	SMSMediaURLs              types.List   `tfsdk:"sms_media_urls"`
}

// pushPlatformFlags maps push_platforms' values to OneSignal's flags.
var pushPlatformFlags = map[string]string{
	"ios":        "isIos",
	"android":    "isAndroid",
	"huawei":     "isHuawei",
	"any_web":    "isAnyWeb",
	"chrome_web": "isChromeWeb",
	"firefox":    "isFirefox",
	"safari":     "isSafari",
	"wp_wns":     "isWP_WNS",
	"adm":        "isAdm",
}

// templateField ties a content attribute to its API name, and converts it.
type templateField struct {
	api  string
	attr func(*templateModel) attr.Value
	set  func(*templateModel, any)
}

func stringField(api string, get func(*templateModel) *types.String) templateField {
	return templateField{api, func(m *templateModel) attr.Value { return *get(m) }, func(m *templateModel, v any) {
		if s, ok := v.(string); ok {
			*get(m) = types.StringValue(s)
		} else {
			*get(m) = types.StringNull()
		}
	}}
}

func mapField(api string, get func(*templateModel) *types.Map) templateField {
	return templateField{api, func(m *templateModel) attr.Value { return *get(m) }, func(m *templateModel, v any) {
		raw, ok := v.(map[string]any)
		if !ok {
			*get(m) = types.MapNull(types.StringType)
			return
		}
		elems := map[string]attr.Value{}
		for k, e := range raw {
			if s, ok := e.(string); ok {
				elems[k] = types.StringValue(s)
			}
		}
		*get(m) = types.MapValueMust(types.StringType, elems)
	}}
}

func listField(api string, get func(*templateModel) *types.List) templateField {
	return templateField{api, func(m *templateModel) attr.Value { return *get(m) }, func(m *templateModel, v any) {
		raw, ok := v.([]any)
		if !ok {
			*get(m) = types.ListNull(types.StringType)
			return
		}
		var elems []attr.Value
		for _, e := range raw {
			if s, ok := e.(string); ok {
				elems = append(elems, types.StringValue(s))
			}
		}
		*get(m) = types.ListValueMust(types.StringType, elems)
	}}
}

var templateFields = []templateField{
	mapField("contents", func(m *templateModel) *types.Map { return &m.Contents }),
	mapField("headings", func(m *templateModel) *types.Map { return &m.Headings }),
	mapField("subtitle", func(m *templateModel) *types.Map { return &m.Subtitle }),
	stringField("url", func(m *templateModel) *types.String { return &m.URL }),
	stringField("app_url", func(m *templateModel) *types.String { return &m.AppURL }),
	stringField("web_url", func(m *templateModel) *types.String { return &m.WebURL }),
	stringField("big_picture", func(m *templateModel) *types.String { return &m.BigPicture }),
	stringField("chrome_web_image", func(m *templateModel) *types.String { return &m.ChromeWebImage }),
	mapField("data", func(m *templateModel) *types.Map { return &m.Data }),
	stringField("email_subject", func(m *templateModel) *types.String { return &m.EmailSubject }),
	stringField("email_body", func(m *templateModel) *types.String { return &m.EmailBody }),
	stringField("email_preheader", func(m *templateModel) *types.String { return &m.EmailPreheader }),
	stringField("email_from_name", func(m *templateModel) *types.String { return &m.EmailFromName }),
	stringField("email_from_address", func(m *templateModel) *types.String { return &m.EmailFromAddress }),
	stringField("email_reply_to_address", func(m *templateModel) *types.String { return &m.EmailReplyToAddress }),
	listField("email_bcc", func(m *templateModel) *types.List { return &m.EmailBCC }),
	{"disable_email_click_tracking", func(m *templateModel) attr.Value { return m.DisableEmailClickTracking }, func(m *templateModel, v any) {
		if b, ok := v.(bool); ok {
			m.DisableEmailClickTracking = types.BoolValue(b)
		} else {
			m.DisableEmailClickTracking = types.BoolNull()
		}
	}},
	stringField("sms_from", func(m *templateModel) *types.String { return &m.SMSFrom }),
	listField("sms_media_urls", func(m *templateModel) *types.List { return &m.SMSMediaURLs }),
}

func (r *TemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template"
}

func (r *TemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	stringMap := func(desc string) schema.MapAttribute {
		return schema.MapAttribute{MarkdownDescription: desc, ElementType: types.StringType, Optional: true}
	}
	stringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{MarkdownDescription: desc, ElementType: types.StringType, Optional: true}
	}
	platforms := make([]string, 0, len(pushPlatformFlags))
	for p := range pushPlatformFlags {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	resp.Schema = schema.Schema{
		MarkdownDescription: "A message template of an app, for one channel. A field OneSignal does not return keeps its configured value, so changes made outside OpenTofu show only for the fields it returns.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The template's ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"app_id": schema.StringAttribute{
				MarkdownDescription: "The app the template belongs to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"api_key": appKeyAttribute(),
			"name": schema.StringAttribute{
				MarkdownDescription: "The template's name.",
				Required:            true,
			},
			"channel": schema.StringAttribute{
				MarkdownDescription: "`push`, `email` or `sms`. Changing it replaces the template.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(onesignal.ChannelPush, onesignal.ChannelEmail, onesignal.ChannelSMS)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"contents":         stringMap("The message, by language code; push and SMS need `en`."),
			"headings":         stringMap("Push: the title, by language code."),
			"subtitle":         stringMap("Push: the subtitle, by language code."),
			"url":              optionalString("Push: the URL opened on tap."),
			"app_url":          optionalString("Push: the URL opened on tap in the mobile apps."),
			"web_url":          optionalString("Push: the URL opened on tap in browsers."),
			"big_picture":      optionalString("Push: Android's large image."),
			"chrome_web_image": optionalString("Push: Chrome's large image."),
			"data":             stringMap("Push: custom data delivered with the notification."),
			"push_platforms": schema.SetAttribute{
				MarkdownDescription: "Push: the platforms targeted. Unset, every platform is.",
				ElementType:         types.StringType,
				Optional:            true,
				Validators:          []validator.Set{setvalidator.ValueStringsAre(stringvalidator.OneOf(platforms...))},
			},
			"email_subject":          optionalString("Email: the subject."),
			"email_body":             optionalString("Email: the HTML body."),
			"email_preheader":        optionalString("Email: the preview text."),
			"email_from_name":        optionalString("Email: the sender's name."),
			"email_from_address":     optionalString("Email: the sender's address."),
			"email_reply_to_address": optionalString("Email: the reply-to address."),
			"email_bcc":              stringList("Email: up to five BCC addresses, with OneSignal's own email service."),
			"disable_email_click_tracking": schema.BoolAttribute{
				MarkdownDescription: "Email: whether link clicks go untracked.",
				Optional:            true,
			},
			"sms_from":       optionalString("SMS: the sender, a Messaging Service SID or an E.164 number."),
			"sms_media_urls": stringList("SMS: media URLs, which make it an MMS."),
		},
	}
}

func (r *TemplateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m templateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Channel.IsUnknown() || m.Channel.IsNull() {
		return
	}
	switch m.Channel.ValueString() {
	case onesignal.ChannelPush, onesignal.ChannelSMS:
		if m.Contents.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("contents"), "Missing contents", "A push or SMS template needs `contents`, with at least `en`.")
		} else if !m.Contents.IsUnknown() {
			if _, ok := m.Contents.Elements()["en"]; !ok {
				resp.Diagnostics.AddAttributeError(path.Root("contents"), "Missing English contents", "OneSignal requires `contents` to have an `en` entry.")
			}
		}
	case onesignal.ChannelEmail:
		if m.EmailSubject.IsNull() || m.EmailBody.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("email_body"), "Missing email content", "An email template needs `email_subject` and `email_body`.")
		}
	}
}

func (r *TemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// toAPI converts a known attribute value to what OneSignal takes.
func toAPI(ctx context.Context, v attr.Value) any {
	switch t := v.(type) {
	case types.String:
		return t.ValueString()
	case types.Bool:
		return t.ValueBool()
	case types.Map:
		out := map[string]string{}
		t.ElementsAs(ctx, &out, false)
		return out
	case types.List:
		var out []string
		t.ElementsAs(ctx, &out, false)
		return out
	}
	return nil
}

// templateFieldsOf returns what plan sets; with state given, fields set there
// and removed from plan are sent as null, to clear them.
func templateFieldsOf(ctx context.Context, plan, state *templateModel) onesignal.TemplateFields {
	fields := onesignal.TemplateFields{}
	for _, f := range templateFields {
		v := f.attr(plan)
		switch {
		case !v.IsNull() && !v.IsUnknown():
			fields[f.api] = toAPI(ctx, v)
		case state != nil && !f.attr(state).IsNull():
			fields[f.api] = nil
		}
	}
	if plan.Channel.ValueString() == onesignal.ChannelPush && !plan.PushPlatforms.IsNull() && !plan.PushPlatforms.IsUnknown() {
		var chosen []string
		plan.PushPlatforms.ElementsAs(ctx, &chosen, false)
		for _, p := range chosen {
			fields[pushPlatformFlags[p]] = true
		}
	}
	return fields
}

// setFromTemplate copies a template into m. A field absent from OneSignal's
// response keeps m's value; push_platforms is read only when m sets it.
func setFromTemplate(m *templateModel, t *onesignal.Template) {
	m.ID = types.StringValue(t.ID)
	m.Name = types.StringValue(t.Name)
	m.Channel = types.StringValue(t.NormalizedChannel())
	for _, f := range templateFields {
		if v, ok := t.Content[f.api]; ok {
			f.set(m, v)
		}
	}
	if !m.PushPlatforms.IsNull() && m.Channel.ValueString() == onesignal.ChannelPush {
		var chosen []attr.Value
		for p, flag := range pushPlatformFlags {
			if b, _ := t.Content[flag].(bool); b {
				chosen = append(chosen, types.StringValue(p))
			}
		}
		m.PushPlatforms = types.SetValueMust(types.StringType, chosen)
	}
}

func (r *TemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan templateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.CreateTemplate(ctx, plan.APIKey.ValueString(), plan.AppID.ValueString(), plan.Name.ValueString(), plan.Channel.ValueString(), templateFieldsOf(ctx, &plan, nil))
	if err != nil {
		resp.Diagnostics.AddError("Creating the template", err.Error())
		return
	}
	setFromTemplate(&plan, t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state templateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetTemplate(ctx, state.APIKey.ValueString(), state.AppID.ValueString(), state.ID.ValueString())
	if onesignal.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading the template", err.Error())
		return
	}
	setFromTemplate(&state, t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state templateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, appID, id := plan.APIKey.ValueString(), state.AppID.ValueString(), state.ID.ValueString()
	if err := r.client.UpdateTemplate(ctx, key, appID, id, plan.Name.ValueString(), plan.Channel.ValueString(), templateFieldsOf(ctx, &plan, &state)); err != nil {
		resp.Diagnostics.AddError("Updating the template", err.Error())
		return
	}
	t, err := r.client.GetTemplate(ctx, key, appID, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading the updated template", err.Error())
		return
	}
	setFromTemplate(&plan, t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state templateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteTemplate(ctx, state.APIKey.ValueString(), state.AppID.ValueString(), state.ID.ValueString())
	if err != nil && !onesignal.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting the template", err.Error())
	}
}

func (r *TemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	appID, id, ok := splitImportID(req.ID, "template", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app_id"), appID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
