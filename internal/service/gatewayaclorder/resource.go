package gatewayaclorder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"terraform-provider-omada/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/Tohaker/omada-go-sdk/omada"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &gatewayAclOrderResource{}
	_ resource.ResourceWithConfigure   = &gatewayAclOrderResource{}
	_ resource.ResourceWithImportState = &gatewayAclOrderResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &gatewayAclOrderResource{}
}

// gatewayAclOrderResource is the resource implementation.
type gatewayAclOrderResource struct {
	gatewayAclOrderClient
}

// Configure adds the provider configured client to the resource.
func (r *gatewayAclOrderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *gatewayAclOrderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gateway_acl_order"
}

// Schema defines the schema for the resource.
func (r *gatewayAclOrderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the site-global evaluation order of Omada gateway (OSG) ACL rules. Omada " +
			"evaluates gateway ACLs top-down and reordering is a per-device-type, whole-map operation " +
			"(POST /acls/modifyIndex, type \"gateway\"), so this dedicated singleton owns ordering instead " +
			"of individual omada_acl resources fighting for absolute indexes. `ordered_acl_ids` must list " +
			"every gateway ACL id on the site, highest priority (evaluated first) first; a missing or extra " +
			"id is rejected so an out-of-band rule is surfaced, never silently reordered. There is one of " +
			"these resources per site. Requires one of: `Site Settings Manager Modify` or `Network Config " +
			"Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				Description: "Site ID whose gateway ACL order this resource owns. Also the import ID. " +
					"Changing this forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ordered_acl_ids": schema.ListAttribute{
				Description: "Gateway ACL ids in the desired evaluation order — highest priority (evaluated " +
					"first, top of the list, index 1) first. Must be exhaustive: exactly the set of gateway " +
					"ACL ids on the site. Reference omada_acl.<name>.acl_id so ordering follows the managed " +
					"rules. Read reflects the live order here, so out-of-band reorders or added/removed ACLs " +
					"appear as a plan diff.",
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

// decodeEnvelope reads the (re-readable) response body from an SDK call and
// decodes the standard Omada envelope leniently.
func decodeEnvelope(httpResp *http.Response, callErr error, diags *diag.Diagnostics, action string) (omadaEnvelope, bool) {
	if callErr != nil && httpResp == nil {
		diags.AddError("Error "+action, "Transport error: "+callErr.Error())
		return omadaEnvelope{}, false
	}
	if httpResp == nil {
		diags.AddError("Error "+action, "Controller returned no response.")
		return omadaEnvelope{}, false
	}
	defer httpResp.Body.Close()

	body, readErr := io.ReadAll(httpResp.Body)
	if readErr != nil {
		diags.AddError("Error "+action, "Could not read response body: "+readErr.Error())
		return omadaEnvelope{}, false
	}

	var env omadaEnvelope
	if jsonErr := json.Unmarshal(body, &env); jsonErr != nil {
		msg := "Could not decode response: " + jsonErr.Error()
		if callErr != nil {
			msg += fmt.Sprintf(" (original error: %s)", callErr.Error())
		}
		diags.AddError("Error "+action, msg)
		return omadaEnvelope{}, false
	}

	// A transport/HTTP error whose body decoded but carries no errorCode would
	// otherwise slip past hasError() and be treated as success; surface it.
	if callErr != nil && env.ErrorCode == nil {
		diags.AddError("Error "+action, "API call failed: "+callErr.Error())
		return omadaEnvelope{}, false
	}

	return env, true
}

// Create sets the gateway ACL order and records it in state.
func (r *gatewayAclOrderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gatewayAclOrderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.applyOrder(ctx, &resp.Diagnostics, &plan) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes state with the live gateway ACL order. The controller-assigned
// order is read from the paged gateway ACL list sorted by index, so an
// out-of-band reorder or a created/deleted ACL surfaces as a plan diff.
func (r *gatewayAclOrderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gatewayAclOrderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ordered, ok := r.liveOrder(ctx, &resp.Diagnostics, state.SiteId.ValueString())
	if !ok {
		return
	}
	state.OrderedAclIds = ordered

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update re-applies the desired gateway ACL order.
func (r *gatewayAclOrderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gatewayAclOrderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.applyOrder(ctx, &resp.Diagnostics, &plan) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete stops managing the order. Ordering is intrinsic to the ACLs (there is
// nothing to remove on the controller), so Delete is a no-op beyond dropping the
// resource from state.
func (r *gatewayAclOrderResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState imports the per-site order. The import ID is the site_id; the
// framework follows with a Read that populates ordered_acl_ids from the live
// order.
func (r *gatewayAclOrderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.TrimSpace(req.ID) == "" {
		resp.Diagnostics.AddError("Unexpected Import ID", "Expected the site_id as the import ID.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), req.ID)...)
}

// applyOrder validates that the planned ordered_acl_ids is exhaustive against the
// live gateway ACL set, then reorders via ModifyAclIndex(type="gateway"). On
// success the planned order is authoritative (the reorder was accepted), so the
// caller records plan into state directly.
func (r *gatewayAclOrderResource) applyOrder(ctx context.Context, diags *diag.Diagnostics, plan *gatewayAclOrderResourceModel) bool {
	siteId := plan.SiteId.ValueString()

	liveIDs, ok := r.liveIDSet(ctx, diags, siteId)
	if !ok {
		return false
	}

	// Build the desired order and validate it is an exact, duplicate-free cover
	// of the live gateway ACL set. Refusing a non-exhaustive list is what keeps
	// the reorder from silently clobbering out-of-band rules.
	want := make([]string, 0, len(plan.OrderedAclIds))
	seen := make(map[string]bool, len(plan.OrderedAclIds))
	for _, v := range plan.OrderedAclIds {
		id := v.ValueString()
		if seen[id] {
			diags.AddError("Invalid gateway ACL order", fmt.Sprintf("ordered_acl_ids contains duplicate id %q.", id))
			return false
		}
		seen[id] = true
		if !liveIDs[id] {
			diags.AddError(
				"Invalid gateway ACL order",
				fmt.Sprintf("ordered_acl_ids references gateway ACL %q, which does not exist on site %s.", id, siteId),
			)
			return false
		}
		want = append(want, id)
	}
	var missing []string
	for id := range liveIDs {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		diags.AddError(
			"Gateway ACL order is not exhaustive",
			fmt.Sprintf("gateway ACL(s) %s exist on site %s but are not listed in ordered_acl_ids. "+
				"Add every gateway ACL id (order is site-global), or remove the rule.", strings.Join(missing, ", "), siteId),
		)
		return false
	}

	// Assign 1-based contiguous indexes matching list position (index 1 = first
	// evaluated), the numbering the controller uses.
	indexes := make(map[string]int32, len(want))
	for i, id := range want {
		indexes[id] = int32(i + 1)
	}

	_, httpResp, callErr := r.client.ACLAPI.ModifyAclIndex(ctx, r.omadacId, siteId).
		DragSortIndexOpenapiVO(omada.DragSortIndexOpenapiVO{Indexes: indexes, Type: dragSortTypeGateway}).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reordering gateway ACLs")
	if !ok {
		return false
	}
	if env.hasError() {
		respondAPIError(diags, "reordering gateway ACLs", env.ErrorCode, env.Msg)
		return false
	}

	return true
}

// fetchList returns the paged gateway ACL list for the site, decoded leniently.
func (r *gatewayAclOrderResource) fetchList(ctx context.Context, diags *diag.Diagnostics, siteId string) []aclIndexRow {
	_, httpResp, callErr := r.client.ACLAPI.GetOsgAclList(ctx, r.omadacId, siteId).
		Page(1).PageSize(1000).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading gateway ACLs")
	if !ok {
		return nil
	}
	if env.hasError() {
		diags.AddError(
			"Error reading gateway ACLs",
			fmt.Sprintf("Controller rejected the list for site %s, error code %d: %s", siteId, *env.ErrorCode, env.Msg),
		)
		return nil
	}

	var lr listResult
	if err := json.Unmarshal(env.Result, &lr); err != nil {
		diags.AddError("Error reading gateway ACLs", "Could not decode gateway ACL list: "+err.Error())
		return nil
	}
	return lr.Data
}

// liveOrder returns the live gateway ACL ids sorted by controller index
// (ascending = evaluation order).
func (r *gatewayAclOrderResource) liveOrder(ctx context.Context, diags *diag.Diagnostics, siteId string) ([]types.String, bool) {
	rows := r.fetchList(ctx, diags, siteId)
	if diags.HasError() {
		return nil, false
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Index < rows[j].Index })
	out := make([]types.String, 0, len(rows))
	for _, row := range rows {
		out = append(out, types.StringValue(row.Id))
	}
	return out, true
}

// liveIDSet returns the set of gateway ACL ids currently on the site.
func (r *gatewayAclOrderResource) liveIDSet(ctx context.Context, diags *diag.Diagnostics, siteId string) (map[string]bool, bool) {
	rows := r.fetchList(ctx, diags, siteId)
	if diags.HasError() {
		return nil, false
	}
	set := make(map[string]bool, len(rows))
	for _, row := range rows {
		set[row.Id] = true
	}
	return set, true
}

// respondAPIError records a controller-side error (non-zero errorCode).
func respondAPIError(diags *diag.Diagnostics, action string, code *int32, msg string) {
	if code == nil {
		diags.AddError("Error "+action, "Controller rejected the request: "+msg)
		return
	}
	diags.AddError("Error "+action, fmt.Sprintf("Controller rejected the request, error code %d: %s", *code, msg))
}
