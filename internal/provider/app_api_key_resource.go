// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
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
	_ resource.Resource                = &AppAPIKeyResource{}
	_ resource.ResourceWithImportState = &AppAPIKeyResource{}
)

// NewAppAPIKeyResource returns the onesignal_app_api_key resource.
func NewAppAPIKeyResource() resource.Resource { return &AppAPIKeyResource{} }

// AppAPIKeyResource is an App API key.
type AppAPIKeyResource struct {
	client *onesignal.Client
}

type appAPIKeyModel struct {
	ID                types.String `tfsdk:"id"`
	AppID             types.String `tfsdk:"app_id"`
	Name              types.String `tfsdk:"name"`
	IPAllowlist       types.Set    `tfsdk:"ip_allowlist"`
	RotateWhenChanged types.Map    `tfsdk:"rotate_when_changed"`
	Value             types.String `tfsdk:"value"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

func (r *AppAPIKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_api_key"
}

// cidrValidator accepts IPv4 and IPv6 CIDR ranges.
type cidrValidator struct{}

func (cidrValidator) Description(context.Context) string { return "must be a CIDR range" }
func (v cidrValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (cidrValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, _, err := net.ParseCIDR(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid CIDR range", fmt.Sprintf("%q is not a CIDR range, such as 192.0.2.10/32 or 2001:db8::/64.", req.ConfigValue.ValueString()))
	}
}

// rotatedValue plans a new value when rotate_when_changed changes, and keeps the
// stored one otherwise.
type rotatedValue struct{}

func (rotatedValue) Description(context.Context) string {
	return "Becomes unknown when rotate_when_changed changes."
}
func (v rotatedValue) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (rotatedValue) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() {
		return // Created: the value is known after apply.
	}
	var planned, stored types.Map
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("rotate_when_changed"), &planned)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("rotate_when_changed"), &stored)...)
	if planned.Equal(stored) {
		resp.PlanValue = req.StateValue
		return
	}
	resp.PlanValue = types.StringUnknown()
}

func (r *AppAPIKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An App API key. OneSignal returns a key's value only when it is created or rotated, so `value` is captured then and kept in state; an imported key has none. " +
			"Changing `rotate_when_changed` rotates the key in place: same ID, new value, the old one revoked at once. Destroying it revokes it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The key's ID, OneSignal's `token_id`. It is not the key.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"app_id": schema.StringAttribute{
				MarkdownDescription: "The app the key belongs to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The key's name.",
				Required:            true,
			},
			"ip_allowlist": schema.SetAttribute{
				MarkdownDescription: "The CIDR ranges, IPv4 or IPv6, the key may be used from. Unset or empty, it may be used from anywhere.",
				ElementType:         types.StringType,
				Optional:            true,
				Validators:          []validator.Set{setvalidator.ValueStringsAre(cidrValidator{})},
			},
			"rotate_when_changed": schema.MapAttribute{
				MarkdownDescription: "Arbitrary values; any change rotates the key.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The key itself, from its creation or last rotation.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers:       []planmodifier.String{rotatedValue{}},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the key was created.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *AppAPIKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func allowlistOf(ctx context.Context, set types.Set) []string {
	var list []string
	if !set.IsNull() && !set.IsUnknown() {
		set.ElementsAs(ctx, &list, false)
	}
	return list
}

// setFromAPIKey copies what OneSignal returns into m. An empty allowlist stays
// empty or null as the model had it, so neither shows as drift.
func setFromAPIKey(ctx context.Context, m *appAPIKeyModel, k *onesignal.APIKey) {
	m.ID = types.StringValue(k.TokenID)
	m.Name = types.StringValue(k.Name)
	m.CreatedAt = types.StringValue(k.CreatedAt)
	if k.IPAllowlistMode == onesignal.AllowlistExplicit && len(k.IPAllowlist) > 0 {
		m.IPAllowlist, _ = types.SetValueFrom(ctx, types.StringType, k.IPAllowlist)
	} else if !m.IPAllowlist.IsNull() && !m.IPAllowlist.IsUnknown() {
		m.IPAllowlist, _ = types.SetValueFrom(ctx, types.StringType, []string{})
	} else {
		m.IPAllowlist = types.SetNull(types.StringType)
	}
}

func (r *AppAPIKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan appAPIKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := onesignal.NewAPIKeyInput(plan.Name.ValueString(), allowlistOf(ctx, plan.IPAllowlist))
	created, err := r.client.CreateAPIKey(ctx, plan.AppID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Creating the API key", err.Error())
		return
	}
	plan.Value = types.StringValue(created.FormattedToken)
	plan.ID = types.StringValue(created.TokenID)
	key, err := r.client.GetAPIKey(ctx, plan.AppID.ValueString(), created.TokenID)
	if err != nil {
		resp.Diagnostics.AddError("Reading the new API key", err.Error())
		// Keep the key and its value in state, so it is not orphaned.
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}
	setFromAPIKey(ctx, &plan, key)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppAPIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state appAPIKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, err := r.client.GetAPIKey(ctx, state.AppID.ValueString(), state.ID.ValueString())
	if onesignal.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading the API key", err.Error())
		return
	}
	setFromAPIKey(ctx, &state, key)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AppAPIKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state appAPIKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	appID, id := state.AppID.ValueString(), state.ID.ValueString()
	if !plan.Name.Equal(state.Name) || !plan.IPAllowlist.Equal(state.IPAllowlist) {
		in := onesignal.NewAPIKeyInput(plan.Name.ValueString(), allowlistOf(ctx, plan.IPAllowlist))
		if err := r.client.UpdateAPIKey(ctx, appID, id, in); err != nil {
			resp.Diagnostics.AddError("Updating the API key", err.Error())
			return
		}
	}
	plan.Value = state.Value
	if !plan.RotateWhenChanged.Equal(state.RotateWhenChanged) {
		value, err := r.client.RotateAPIKey(ctx, appID, id)
		if err != nil {
			resp.Diagnostics.AddError("Rotating the API key", err.Error())
			return
		}
		plan.Value = types.StringValue(value)
	}
	key, err := r.client.GetAPIKey(ctx, appID, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading the updated API key", err.Error())
		return
	}
	setFromAPIKey(ctx, &plan, key)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppAPIKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state appAPIKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteAPIKey(ctx, state.AppID.ValueString(), state.ID.ValueString())
	if err != nil && !onesignal.IsNotFound(err) {
		resp.Diagnostics.AddError("Revoking the API key", err.Error())
	}
}

func (r *AppAPIKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	appID, id, ok := splitImportID(req.ID, "api_key", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app_id"), appID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.AddWarning("The imported key has no value",
		"OneSignal returns a key's value only when it is created or rotated, so `value` stays null until the key is rotated through rotate_when_changed.")
}
