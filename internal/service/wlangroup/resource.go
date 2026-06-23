package wlangroup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"terraform-provider-omada/internal/client"
	"time"

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

// decodeEnvelope reads the (re-readable) response body from an SDK call and
// decodes the standard Omada envelope leniently. This sidesteps the SDK's
// strict per-model decoders and recovers the controller's errorCode/msg.
func decodeEnvelope(httpResp *http.Response, callErr error, diags *diag.Diagnostics, action string) (omadaEnvelope, bool) {
	if callErr != nil && httpResp == nil {
		diags.AddError("Error "+action, "Transport error: "+callErr.Error())
		return omadaEnvelope{}, false
	}
	if httpResp == nil {
		diags.AddError("Error "+action, "Controller returned no response.")
		return omadaEnvelope{}, false
	}
	// The generated SDK already drains + NopCloser-rewraps Body before returning
	// it, so Close() here is a defensive no-op; kept for hygiene and robustness
	// against future SDK changes.
	defer httpResp.Body.Close()

	body, readErr := io.ReadAll(httpResp.Body)
	if readErr != nil {
		diags.AddError("Error "+action, "Could not read response body: "+readErr.Error())
		return omadaEnvelope{}, false
	}

	var env omadaEnvelope
	if jsonErr := json.Unmarshal(body, &env); jsonErr != nil {
		diags.AddError("Error "+action, "Could not decode response: "+jsonErr.Error())
		return omadaEnvelope{}, false
	}

	return env, true
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
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating WLAN group")
	if !ok {
		return
	}

	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating WLAN group", env.ErrorCode, env.Msg)
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
	if plan.WlanId.IsNull() && !awaitFindWlanGroupByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating WLAN group",
			"Create did not return an id and the group was not present in the site afterwards.",
		)
		return
	}

	if !awaitReadWlanGroup(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating WLAN group",
			"The group was created but could not be read back within the retry window.",
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

	readWlanGroup(ctx, &resp.Diagnostics, r, &state)

	// When the group is gone upstream, readWlanGroup clears WlanId; leave
	// resp.State unset so Terraform drops it from state.
	if state.WlanId.IsNull() {
		return
	}

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
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating WLAN group")
	if !ok {
		return
	}

	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating WLAN group", env.ErrorCode, env.Msg)
		return
	}

	if !readWlanGroup(ctx, &resp.Diagnostics, r, &plan) {
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
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "deleting WLAN group")
	if !ok {
		return
	}

	// -1001 (invalid request parameters) is returned when the group no longer
	// exists on 5.15.x; that is the desired end state for a delete.
	if env.hasError() && env.ErrorCode != nil && *env.ErrorCode != errNotFound {
		respondAPIError(&resp.Diagnostics, "deleting WLAN group", env.ErrorCode, env.Msg)
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
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading WLAN group")
	if !ok {
		return nil
	}

	if env.hasError() {
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
// a single immediate read can miss a group that was just created.
func awaitFindWlanGroupByName(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) bool {
	return runWithBackoff(ctx, func() bool {
		return findWlanGroupByName(ctx, diags, r, model)
	})
}

// readWlanGroup selects the list entry matching the model's wlan_group_id and
// refreshes the model in place. When the group no longer exists, it clears
// model.WlanId so the caller can drop the resource from state.
func readWlanGroup(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) bool {
	rows := fetchWlanGroupList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range rows {
		if rows[i].WlanId != nil && *rows[i].WlanId == model.WlanId.ValueString() {
			flattenWlanGroupRead(model, &rows[i])
			return true
		}
	}

	// Not present in the list: the group is gone upstream.
	model.WlanId = types.StringNull()
	return true
}

// awaitReadWlanGroup retries the list-based read briefly after create so the
// just-created group is reflected despite propagation lag. Unlike readWlanGroup
// it never clears the id (the group is known to exist; it just has not appeared
// in the list yet). Returns false only if it never becomes visible.
func awaitReadWlanGroup(ctx context.Context, diags *diag.Diagnostics, r *wlanGroupResource, model *wlanGroupResourceModel) bool {
	return runWithBackoff(ctx, func() bool {
		rows := fetchWlanGroupList(ctx, diags, r, model)
		if diags.HasError() {
			return false
		}
		for i := range rows {
			if rows[i].WlanId != nil && *rows[i].WlanId == model.WlanId.ValueString() {
				flattenWlanGroupRead(model, &rows[i])
				return true
			}
		}
		return false
	})
}

// readAttempts/readInterval bound the post-create read retry. The controller is
// expected to reflect a create within a few seconds; this is a safety margin,
// not a long poll.
const (
	readAttempts = 10
	readInterval = 750 * time.Millisecond
)

// runWithBackoff repeats fn until it returns true or the attempt budget is
// exhausted. It honors context cancellation between attempts.
func runWithBackoff(ctx context.Context, fn func() bool) bool {
	for attempt := 0; attempt < readAttempts; attempt++ {
		if fn() {
			return true
		}
		select {
		case <-time.After(readInterval):
		case <-ctx.Done():
			return false
		}
	}
	return false
}

// respondAPIError records a controller-side error (non-zero errorCode) on the
// given diagnostics.
func respondAPIError(diags *diag.Diagnostics, action string, code *int32, msg string) {
	if code == nil {
		diags.AddError("Error "+action, "Controller rejected the request: "+msg)
		return
	}

	diags.AddError(
		"Error "+action,
		fmt.Sprintf("Controller rejected the request, error code %d: %s", *code, msg),
	)
}
