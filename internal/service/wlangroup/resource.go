package wlangroup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"terraform-provider-omada/internal/client"
	"terraform-provider-omada/internal/envelope"
	"terraform-provider-omada/internal/retry"
	"terraform-provider-omada/internal/tfstate"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &wlanGroupResource{}
	_ resource.ResourceWithConfigure   = &wlanGroupResource{}
	_ resource.ResourceWithImportState = &wlanGroupResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &wlanGroupResource{}
}

// wlanGroupResource is the resource implementation.
type wlanGroupResource struct {
	wlanGroupClient
}

// Configure adds the provider configured client to the resource.
func (r *wlanGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*client.Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Meta, got %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = data.Client
	r.omadacId = data.OmadacId
}

// Metadata returns the resource type name.
func (r *wlanGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wlan_group"
}

// Schema defines the schema for the resource.
func (r *wlanGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Omada WLAN group: a named collection of SSIDs that the controller " +
			"binds to access points. Create a WLAN group that is not bound to any AP to stage SSIDs " +
			"safely (they will not broadcast until the group is applied to APs out of band). Targets " +
			"the Open API v1 wireless-network surface implemented by controller firmware such as " +
			"5.15.x. Requires one of: `Site Settings Manager Modify` or `Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"wlan_group_id": schema.StringAttribute{
				Description: "WLAN group ID assigned by the controller. Use it (with site_id) as the import target.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID to create the WLAN group in. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "WLAN group name. Must contain 1 to 128 characters and be unique within the site.",
				Required:    true,
			},
			"primary": schema.BoolAttribute{
				Description: "Whether the controller marks this group as the site's primary (\"Default\") group. " +
					"Computed; this resource never creates or changes the primary group.",
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *wlanGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wlanGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WirelessNetworkAPI.CreateWlanGroup(ctx, r.omadacId, plan.SiteId.ValueString()).
		CreateWlanGroupOpenApiVO(expandCreateWlanGroup(plan.Name.ValueString())).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "creating WLAN group")
	if !ok {
		return
	}

	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "creating WLAN group", env.ErrorCode, env.Msg)
		return
	}

	// Prefer the id from the create result when present; otherwise locate the
	// newly-created group by name (names are unique within a site). The
	// controller's WLAN-group list is eventually consistent, so both lookups
	// retry briefly to tolerate the post-create propagation lag.
	if len(env.Result) > 0 {
		var cr createResult
		if err := json.Unmarshal(env.Result, &cr); err == nil {
			if cr.WlanId != nil && *cr.WlanId != "" {
				plan.WlanId = types.StringValue(*cr.WlanId)
			} else if cr.Id != nil && *cr.Id != "" {
				plan.WlanId = types.StringValue(*cr.Id)
			}
		}
	}

	// wlan_group_id is Computed, so it is Unknown (not Null) at create time;
	// testing only IsNull skipped the name lookup and read back an empty id.
	if (plan.WlanId.IsUnknown() || plan.WlanId.IsNull()) && !awaitFindWlanGroupByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating WLAN group",
			"Create did not return an id and the group was not present in the site afterwards.",
		)
		return
	}

	if !awaitReadWlanGroup(ctx, &resp.Diagnostics, r, &plan) {
		// The group exists on the controller: keep it in state (tainted) rather
		// than orphaning it, so a re-apply replaces it instead of failing on a
		// duplicate name.
		resp.Diagnostics.Append(tfstate.SaveCreated(ctx, req.Plan, &resp.State, "wlan_group_id", plan.WlanId.ValueString())...)
		resp.Diagnostics.AddError(
			"Error creating WLAN group",
			fmt.Sprintf("WLAN group %s was created but could not be read back within the retry window. It is kept "+
				"in state as tainted, so the next apply replaces it.", plan.WlanId.ValueString()),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data. The controller
// exposes no single-group GET on the v1 surface, so Read fetches the group
// list and selects the entry matching wlan_group_id.
func (r *wlanGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wlanGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	row, found := findWlanGroupConfirmed(ctx, &resp.Diagnostics, r, &state)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		// Deleted outside Terraform. Setting nothing would keep the prior state,
		// which the framework pre-fills, so the group would silently stay
		// "managed".
		resp.State.RemoveResource(ctx)
		return
	}
	flattenWlanGroupRead(&state, row)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *wlanGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan wlanGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state wlanGroupResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.WlanId = state.WlanId
	plan.SiteId = state.SiteId

	_, httpResp, callErr := r.client.WirelessNetworkAPI.UpdateWlanGroup(ctx, r.omadacId, plan.SiteId.ValueString(), plan.WlanId.ValueString()).
		UpdateWlanGroupOpenApiVO(expandUpdateWlanGroup(plan.Name.ValueString())).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "updating WLAN group")
	if !ok {
		return
	}

	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "updating WLAN group", env.ErrorCode, env.Msg)
		return
	}

	if !awaitReadWlanGroup(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error updating WLAN group",
			fmt.Sprintf("WLAN group %s was updated but could not be read back within the retry window.", plan.WlanId.ValueString()),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *wlanGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wlanGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WirelessNetworkAPI.DeleteWlanGroup(ctx, r.omadacId, state.SiteId.ValueString(), state.WlanId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "deleting WLAN group")
	if !ok {
		return
	}

	// A rejected delete of a group that is already gone is the desired end
	// state. The controller answers a missing group with -1001, which is also
	// its generic "invalid request parameters", so a code alone once dropped a
	// live group from state; confirm against the list instead.
	if env.HasError() {
		var listDiags diag.Diagnostics
		if _, found := findWlanGroupConfirmed(ctx, &listDiags, r, &state); !listDiags.HasError() && !found {
			return
		}
		envelope.AddAPIError(&resp.Diagnostics, "deleting WLAN group", env.ErrorCode, env.Msg)
		return
	}
}

// ImportState imports an existing WLAN group. The import ID is
// `<site_id>/<wlan_group_id>`. The framework follows ImportState with a Read,
// which fully populates the remaining attributes from the controller.
func (r *wlanGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<wlan_group_id>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wlan_group_id"), idParts[1])...)
}

// fetchWlanGroupList fetches the WLAN-group list for the model's site and
// decodes it leniently, tolerating both the bare-array and paged shapes.
func fetchWlanGroupList(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) []wlanGroupReadRow {
	_, httpResp, callErr := r.client.WirelessNetworkAPI.GetWlanGroupList(ctx, r.omadacId, model.SiteId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading WLAN group")
	if !ok {
		return nil
	}

	if env.HasError() {
		diags.AddError(
			"Error reading WLAN group",
			fmt.Sprintf("Controller rejected the list for site %s, error code %d: %s", model.SiteId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return nil
	}

	rows, err := unwrapList(env.Result)
	if err != nil {
		diags.AddError("Error reading WLAN group", "Could not decode WLAN-group list: "+err.Error())
		return nil
	}

	return rows
}

// findWlanGroupInList fetches the group list and returns the entry matching
// the model's wlan_group_id (nil if not present). It does not mutate the model.
func findWlanGroupInList(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) *wlanGroupReadRow {
	rows := fetchWlanGroupList(ctx, diags, r, model)
	if diags.HasError() {
		return nil
	}
	want := model.WlanId.ValueString()
	for i := range rows {
		if rows[i].WlanId != nil && *rows[i].WlanId == want {
			return &rows[i]
		}
	}
	return nil
}

// findWlanGroupConfirmed looks the model's wlan_group_id up in the list. A
// miss is re-checked a few times before it is believed, because the list is
// eventually consistent; a found row is returned at once.
func findWlanGroupConfirmed(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) (*wlanGroupReadRow, bool) {
	var row *wlanGroupReadRow
	found, last := retry.Until(ctx, goneConfirmations, retry.Interval, func(d *diag.Diagnostics) bool {
		row = findWlanGroupInList(ctx, d, r, model)
		// Stop on an error too: a failing list is not evidence of absence.
		return row != nil || d.HasError()
	})
	diags.Append(last...)
	return row, found && row != nil
}

// goneConfirmations is how many list reads must miss a group before Read or
// Delete treats it as deleted.
const goneConfirmations = 3

// findWlanGroupByName locates the group matching the model's name (used after
// create when the id is not returned). Sets model.WlanId on success; returns
// false if not found.
func findWlanGroupByName(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) bool {
	rows := fetchWlanGroupList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range rows {
		if rows[i].Name == model.Name.ValueString() {
			model.WlanId = types.StringPointerValue(rows[i].WlanId)
			return true
		}
	}

	return false
}

// awaitFindWlanGroupByName retries findWlanGroupByName briefly. The
// controller's WLAN-group list is eventually consistent right after create, so
// a single immediate read can miss a group that was just created. Only the
// last attempt's diagnostics are kept, so a transient error doesn't fail the
// apply.
func awaitFindWlanGroupByName(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		return findWlanGroupByName(ctx, d, r, model)
	})
	diags.Append(last...)
	return ok
}

// awaitReadWlanGroup retries the list read until the group is present,
// refreshing the model in place. The list is eventually consistent, so a
// freshly created or renamed group may take a moment to appear. It never
// clears wlan_group_id: the caller (Create/Update) already knows the id. Only
// the last attempt's diagnostics are kept, so a transient error doesn't fail
// the apply.
func awaitReadWlanGroup(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		if row := findWlanGroupInList(ctx, d, r, model); row != nil {
			flattenWlanGroupRead(model, row)
			return true
		}
		return false
	})
	diags.Append(last...)
	return ok
}
