// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
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
	ID                  types.String `tfsdk:"id"`
	AppID               types.String `tfsdk:"app_id"`
	APIKey              types.String `tfsdk:"api_key"`
	Name                types.String `tfsdk:"name"`
	Channel             types.String `tfsdk:"channel"`
	Contents            types.Map    `tfsdk:"contents"`
	Headings            types.Map    `tfsdk:"headings"`
	Subtitle            types.Map    `tfsdk:"subtitle"`
	URL                 types.String `tfsdk:"url"`
	GlobalImage         types.String `tfsdk:"global_image"`
	PushPlatforms       types.Set    `tfsdk:"push_platforms"`
	EmailSubject        types.String `tfsdk:"email_subject"`
	EmailBody           types.String `tfsdk:"email_body"`
	EmailPreheader      types.String `tfsdk:"email_preheader"`
	EmailFromName       types.String `tfsdk:"email_from_name"`
	EmailFromAddress    types.String `tfsdk:"email_from_address"`
	EmailReplyToAddress types.String `tfsdk:"email_reply_to_address"`
	EmailBCC            types.List   `tfsdk:"email_bcc"`
	SMSFrom             types.String `tfsdk:"sms_from"`
	SMSMediaURLs        types.List   `tfsdk:"sms_media_urls"`
}

// pushPlatformFlags maps push_platforms' values to OneSignal's flags: each is
// its flag in snake case, without the "is".
var pushPlatformFlags = map[string]string{
	"ios":        "isIos",
	"android":    "isAndroid",
	"huawei":     "isHuawei",
	"adm":        "isAdm",
	"chrome":     "isChrome",
	"chrome_web": "isChromeWeb",
	"firefox":    "isFirefox",
	"safari":     "isSafari",
	"edge":       "isEdge",
	"macosx":     "isMacOSX",
	"wp":         "isWP",
	"wp_wns":     "isWP_WNS",
	"alexa":      "isAlexa",
}

// templateField ties a content attribute to its API name and the channels it
// belongs to, and converts it.
type templateField struct {
	api      string
	channels []string
	attr     func(*templateModel) attr.Value
	set      func(*templateModel, any)
	// cleared is what clears the field: OneSignal fails on a map sent as null,
	// and clears it when sent {}.
	cleared any
}

func stringField(api string, channels []string, get func(*templateModel) *types.String) templateField {
	return templateField{api, channels, func(m *templateModel) attr.Value { return *get(m) }, func(m *templateModel, v any) {
		if s, ok := v.(string); ok && s != "" {
			*get(m) = types.StringValue(s)
		} else {
			*get(m) = types.StringNull()
		}
	}, nil}
}

func mapField(api string, channels []string, get func(*templateModel) *types.Map) templateField {
	return templateField{api, channels, func(m *templateModel) attr.Value { return *get(m) }, func(m *templateModel, v any) {
		raw, _ := v.(map[string]any)
		elems := map[string]attr.Value{}
		for k, e := range raw {
			if s, ok := e.(string); ok {
				elems[k] = types.StringValue(s)
			}
		}
		if len(elems) == 0 {
			*get(m) = types.MapNull(types.StringType)
			return
		}
		*get(m) = types.MapValueMust(types.StringType, elems)
	}, map[string]string{}}
}

func listField(api string, channels []string, get func(*templateModel) *types.List) templateField {
	return templateField{api, channels, func(m *templateModel) attr.Value { return *get(m) }, func(m *templateModel, v any) {
		raw, _ := v.([]any)
		var elems []attr.Value
		for _, e := range raw {
			if s, ok := e.(string); ok {
				elems = append(elems, types.StringValue(s))
			}
		}
		if len(elems) == 0 {
			*get(m) = types.ListNull(types.StringType)
			return
		}
		*get(m) = types.ListValueMust(types.StringType, elems)
	}, nil}
}

var (
	push      = []string{onesignal.ChannelPush}
	email     = []string{onesignal.ChannelEmail}
	sms       = []string{onesignal.ChannelSMS}
	pushOrSMS = []string{onesignal.ChannelPush, onesignal.ChannelSMS}
)

var templateFields = []templateField{
	mapField("contents", pushOrSMS, func(m *templateModel) *types.Map { return &m.Contents }),
	mapField("headings", push, func(m *templateModel) *types.Map { return &m.Headings }),
	mapField("subtitle", push, func(m *templateModel) *types.Map { return &m.Subtitle }),
	stringField("url", push, func(m *templateModel) *types.String { return &m.URL }),
	stringField("global_image", push, func(m *templateModel) *types.String { return &m.GlobalImage }),
	stringField("email_subject", email, func(m *templateModel) *types.String { return &m.EmailSubject }),
	stringField("email_body", email, func(m *templateModel) *types.String { return &m.EmailBody }),
	stringField("email_preheader", email, func(m *templateModel) *types.String { return &m.EmailPreheader }),
	stringField("email_from_name", email, func(m *templateModel) *types.String { return &m.EmailFromName }),
	stringField("email_from_address", email, func(m *templateModel) *types.String { return &m.EmailFromAddress }),
	stringField("email_reply_to_address", email, func(m *templateModel) *types.String { return &m.EmailReplyToAddress }),
	listField("email_bcc", email, func(m *templateModel) *types.List { return &m.EmailBCC }),
	stringField("sms_from", sms, func(m *templateModel) *types.String { return &m.SMSFrom }),
	listField("sms_media_urls", sms, func(m *templateModel) *types.List { return &m.SMSMediaURLs }),
}

func (r *TemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template"
}

func (r *TemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	stringMap := func(desc string) schema.MapAttribute {
		return schema.MapAttribute{MarkdownDescription: desc, ElementType: types.StringType, Optional: true,
			Validators: []validator.Map{mapvalidator.SizeAtLeast(1)}}
	}
	stringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{MarkdownDescription: desc, ElementType: types.StringType, Optional: true,
			Validators: []validator.List{listvalidator.SizeAtLeast(1)}}
	}
	platforms := make([]string, 0, len(pushPlatformFlags))
	for p := range pushPlatformFlags {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	resp.Schema = schema.Schema{
		MarkdownDescription: "A message template of an app, for one channel. An imported template is not read until its first apply, which needs `api_key` and writes the configuration over it.",
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
			"contents":     stringMap("Push and SMS: the message, by language code, with at least `en`."),
			"headings":     stringMap("Push: the title, by language code."),
			"subtitle":     stringMap("Push: the subtitle, by language code."),
			"url":          optionalString("Push: the URL opened on tap."),
			"global_image": optionalString("Push: the URL of an image shown with the notification."),
			"push_platforms": schema.SetAttribute{
				MarkdownDescription: "Push: the platforms targeted, each OneSignal's flag in snake case without its `is`: `" + strings.Join(platforms, "`, `") + "`. Set, every flag is sent, false for those left out. Unset, a new template targets OneSignal's default, web push on Chrome, Edge and Firefox (`chrome_web`, `edge`, `firefox`), and an existing one keeps its platforms.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Set{setvalidator.ValueStringsAre(stringvalidator.OneOf(platforms...))},
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"email_subject":          optionalString("Email: the subject."),
			"email_body":             optionalString("Email: the HTML body."),
			"email_preheader":        optionalString("Email: the preview text."),
			"email_from_name":        optionalString("Email: the sender's name."),
			"email_from_address":     optionalString("Email: the sender's address."),
			"email_reply_to_address": optionalString("Email: the reply-to address."),
			"email_bcc":              stringList("Email: BCC addresses. OneSignal accepts them only on its own email platform."),
			"sms_from":               optionalString("SMS: the sender, a Messaging Service SID or an E.164 number."),
			"sms_media_urls":         stringList("SMS: media URLs, which make it an MMS."),
		},
	}
}

func (r *TemplateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m templateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Channel.IsUnknown() || m.Channel.IsNull() {
		return
	}
	channel := m.Channel.ValueString()
	for _, f := range templateFields {
		if !f.attr(&m).IsNull() && !slices.Contains(f.channels, channel) {
			resp.Diagnostics.AddAttributeError(path.Root(f.api), "Field of another channel", fmt.Sprintf("`%s` is for %s templates, not %s.", f.api, strings.Join(f.channels, " and "), channel))
		}
	}
	if !m.PushPlatforms.IsNull() && channel != onesignal.ChannelPush {
		resp.Diagnostics.AddAttributeError(path.Root("push_platforms"), "Field of another channel", "`push_platforms` is for push templates.")
	}
	switch channel {
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
// and removed from plan are cleared. A known push_platforms sends every flag.
func templateFieldsOf(ctx context.Context, plan, state *templateModel) onesignal.TemplateFields {
	fields := onesignal.TemplateFields{}
	for _, f := range templateFields {
		v := f.attr(plan)
		switch {
		case !v.IsNull() && !v.IsUnknown():
			fields[f.api] = toAPI(ctx, v)
		case state != nil && !f.attr(state).IsNull():
			fields[f.api] = f.cleared
		}
	}
	if plan.Channel.ValueString() == onesignal.ChannelPush && !plan.PushPlatforms.IsNull() && !plan.PushPlatforms.IsUnknown() {
		var chosen []string
		plan.PushPlatforms.ElementsAs(ctx, &chosen, false)
		for p, flag := range pushPlatformFlags {
			fields[flag] = slices.Contains(chosen, p)
		}
	}
	return fields
}

// setFromTemplate copies a template into m. A field absent from OneSignal's
// response keeps m's value; one returned empty is null.
func setFromTemplate(m *templateModel, t *onesignal.Template) {
	m.ID = types.StringValue(t.ID)
	m.Name = types.StringValue(t.Name)
	m.Channel = types.StringValue(t.NormalizedChannel())
	for _, f := range templateFields {
		if v, ok := t.Content[f.api]; ok {
			f.set(m, v)
		}
	}
	if m.Channel.ValueString() != onesignal.ChannelPush {
		m.PushPlatforms = types.SetNull(types.StringType)
		return
	}
	chosen := []attr.Value{}
	for p, flag := range pushPlatformFlags {
		if b, _ := t.Content[flag].(bool); b {
			chosen = append(chosen, types.StringValue(p))
		}
	}
	m.PushPlatforms = types.SetValueMust(types.StringType, chosen)
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
	if resp.Diagnostics.HasError() || importedWithoutKey(state.APIKey) {
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
