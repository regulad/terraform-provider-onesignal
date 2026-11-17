// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal"
)

var (
	_ resource.Resource                   = &SegmentResource{}
	_ resource.ResourceWithImportState    = &SegmentResource{}
	_ resource.ResourceWithValidateConfig = &SegmentResource{}
)

// NewSegmentResource returns the onesignal_segment resource.
func NewSegmentResource() resource.Resource { return &SegmentResource{} }

// SegmentResource is a segment of an app.
type SegmentResource struct {
	client *onesignal.Client
}

type segmentModel struct {
	ID          types.String  `tfsdk:"id"`
	AppID       types.String  `tfsdk:"app_id"`
	APIKey      types.String  `tfsdk:"api_key"`
	Name        types.String  `tfsdk:"name"`
	Description types.String  `tfsdk:"description"`
	Filters     []filterModel `tfsdk:"filters"`
	Source      types.String  `tfsdk:"source"`
}

type filterModel struct {
	Field    types.String `tfsdk:"field"`
	Relation types.String `tfsdk:"relation"`
	Key      types.String `tfsdk:"key"`
	Value    types.String `tfsdk:"value"`
	HoursAgo types.String `tfsdk:"hours_ago"`
	Radius   types.String `tfsdk:"radius"`
	Lat      types.String `tfsdk:"lat"`
	Long     types.String `tfsdk:"long"`
	Operator types.String `tfsdk:"operator"`
}

// filterFields ties each filter attribute to its API key.
var filterFields = []struct {
	api  string
	attr func(*filterModel) *types.String
}{
	{"field", func(f *filterModel) *types.String { return &f.Field }},
	{"relation", func(f *filterModel) *types.String { return &f.Relation }},
	{"key", func(f *filterModel) *types.String { return &f.Key }},
	{"value", func(f *filterModel) *types.String { return &f.Value }},
	{"hours_ago", func(f *filterModel) *types.String { return &f.HoursAgo }},
	{"radius", func(f *filterModel) *types.String { return &f.Radius }},
	{"lat", func(f *filterModel) *types.String { return &f.Lat }},
	{"long", func(f *filterModel) *types.String { return &f.Long }},
	{"operator", func(f *filterModel) *types.String { return &f.Operator }},
}

func (r *SegmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_segment"
}

func optionalString(desc string) schema.StringAttribute {
	return schema.StringAttribute{MarkdownDescription: desc, Optional: true}
}

// appKeyAttribute is the optional App API key of app-level resources.
func appKeyAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "An App API key of the app, such as an `onesignal_app_api_key`'s `value`, for OneSignal's app-level endpoints. Unset, the provider's Organization API key is used.",
		Optional:            true,
		Sensitive:           true,
	}
}

func (r *SegmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A segment of an app, defined by filters. OneSignal's API cannot manage segments built on message-event or custom-event filters, and is not available on every plan.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The segment's ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"app_id": schema.StringAttribute{
				MarkdownDescription: "The app the segment belongs to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"api_key": appKeyAttribute(),
			"name": schema.StringAttribute{
				MarkdownDescription: "The segment's name, unique within the app.",
				Required:            true,
			},
			"description": optionalString("The segment's description."),
			"filters": schema.ListNestedAttribute{
				MarkdownDescription: "The filters, in order: each entry is a filter, or `{ operator = \"OR\" }`. Adjacent filters are joined by AND, which binds tighter than OR, so the list is an OR of AND groups. Every operand is a string, as OneSignal stores it.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"field":     optionalString("What the filter tests: `tag`, `last_session`, `first_session`, `session_count`, `session_time`, `language`, `app_version`, `location` or `country`."),
						"relation":  optionalString("The comparison, such as `=`, `!=`, `>`, `<`, `exists`, `not_exists`, `in_array`, `time_elapsed_gt`."),
						"key":       optionalString("The tag's key, for `tag` filters."),
						"value":     optionalString("The value compared, as a string."),
						"hours_ago": optionalString("Hours, as a string, for `last_session` and `first_session`."),
						"radius":    optionalString("Metres, for `location`."),
						"lat":       optionalString("Latitude, for `location`."),
						"long":      optionalString("Longitude, for `location`."),
						"operator":  optionalString("`OR`, alone in its entry, between two AND groups."),
					},
				},
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "Where the segment came from: `custom`, `default` or `quickstart`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *SegmentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m segmentModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Filters == nil {
		return
	}
	for i, f := range m.Filters {
		p := path.Root("filters").AtListIndex(i)
		op := f.Operator
		if op.IsUnknown() {
			continue
		}
		if !op.IsNull() {
			if strings.ToUpper(op.ValueString()) != "OR" {
				resp.Diagnostics.AddAttributeError(p.AtName("operator"), "Invalid operator", "Only OR is written: adjacent filters are already joined by AND.")
			}
			for _, ff := range filterFields[:len(filterFields)-1] {
				if !ff.attr(&f).IsNull() {
					resp.Diagnostics.AddAttributeError(p, "Invalid filter entry", "An operator entry holds nothing but `operator`.")
					break
				}
			}
			if i == 0 || i == len(m.Filters)-1 {
				resp.Diagnostics.AddAttributeError(p, "Invalid filter entry", "The filters cannot begin or end with an operator.")
			}
			continue
		}
		if f.Field.IsNull() {
			resp.Diagnostics.AddAttributeError(p, "Invalid filter entry", "Each entry is a filter, with a `field`, or an `operator`.")
		}
	}
	if len(m.Filters) > 200 {
		resp.Diagnostics.AddAttributeError(path.Root("filters"), "Too many filters", fmt.Sprintf("OneSignal allows 200 entries; there are %d.", len(m.Filters)))
	}
}

func (r *SegmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func segmentFromModel(m *segmentModel) onesignal.Segment {
	s := onesignal.Segment{Name: m.Name.ValueString()}
	if !m.Description.IsNull() {
		d := m.Description.ValueString()
		s.Description = &d
	}
	for i := range m.Filters {
		f := onesignal.Filter{}
		for _, ff := range filterFields {
			if v := ff.attr(&m.Filters[i]); !v.IsNull() && !v.IsUnknown() {
				f[ff.api] = v.ValueString()
				if ff.api == "operator" {
					f[ff.api] = strings.ToUpper(v.ValueString())
				}
			}
		}
		s.Filters = append(s.Filters, f)
	}
	return s
}

// setFromSegment copies a segment into m, dropping the explicit AND operators
// OneSignal spells out between adjacent filters.
func setFromSegment(m *segmentModel, s *onesignal.Segment) {
	m.ID = types.StringValue(s.ID)
	m.Name = types.StringValue(s.Name)
	if s.Description == nil || *s.Description == "" {
		m.Description = types.StringNull()
	} else {
		m.Description = types.StringValue(*s.Description)
	}
	m.Source = types.StringValue(s.Source)
	m.Filters = nil
	for _, f := range s.Filters {
		if op, ok := f["operator"]; ok && strings.EqualFold(op, "AND") {
			continue
		}
		var fm filterModel
		for _, ff := range filterFields {
			if v, ok := f[ff.api]; ok {
				*ff.attr(&fm) = types.StringValue(v)
			} else {
				*ff.attr(&fm) = types.StringNull()
			}
		}
		m.Filters = append(m.Filters, fm)
	}
}

func (r *SegmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan segmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, appID := plan.APIKey.ValueString(), plan.AppID.ValueString()
	id, err := r.client.CreateSegment(ctx, key, appID, segmentFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating the segment", err.Error())
		return
	}
	seg, err := r.client.GetSegment(ctx, key, appID, id)
	if err != nil {
		// Record the segment anyway, so it is not orphaned.
		plan.ID = types.StringValue(id)
		plan.Source = types.StringNull()
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("Reading the new segment", err.Error())
		return
	}
	setFromSegment(&plan, seg)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SegmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state segmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	seg, err := r.client.GetSegment(ctx, state.APIKey.ValueString(), state.AppID.ValueString(), state.ID.ValueString())
	if onesignal.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading the segment", err.Error())
		return
	}
	setFromSegment(&state, seg)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SegmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state segmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, appID, id := plan.APIKey.ValueString(), state.AppID.ValueString(), state.ID.ValueString()
	if err := r.client.UpdateSegment(ctx, key, appID, id, segmentFromModel(&plan)); err != nil {
		resp.Diagnostics.AddError("Updating the segment", err.Error())
		return
	}
	seg, err := r.client.GetSegment(ctx, key, appID, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading the updated segment", err.Error())
		return
	}
	setFromSegment(&plan, seg)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SegmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state segmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteSegment(ctx, state.APIKey.ValueString(), state.AppID.ValueString(), state.ID.ValueString())
	if err != nil && !onesignal.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting the segment", err.Error())
	}
}

func (r *SegmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	appID, id, ok := splitImportID(req.ID, "segment", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app_id"), appID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
