package ipgroup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"terraform-provider-omada/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// errGroupNotFound codes mark a missing group profile. Delete treats them as a
// successful outcome (the group is already gone), matching how the other
// homelab resources tolerate not-found on delete. Both codes mean "this group
// does not exist" in the Open API error table.
var errGroupNotFound = map[int32]bool{
	-33703: true,
	-33704: true,
}

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &ipGroupResource{}
	_ resource.ResourceWithConfigure   = &ipGroupResource{}
	_ resource.ResourceWithImportState = &ipGroupResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &ipGroupResource{}
}

// ipGroupResource is the resource implementation.
type ipGroupResource struct {
	ipGroupClient
}

// Configure adds the provider configured client to the resource.
func (r *ipGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *ipGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_group"
}

// Schema defines the schema for the resource.
func (r *ipGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Omada IP group profile: a reusable, named set of IP hosts/subnets " +
			"that gateway ACL rules reference by group id (source_type/destination_type = 1) instead " +
			"of hard-coding whole-VLAN networks. Targets the Open API v1 group surface " +
			"(`/openapi/v1/.../profiles/groups`) implemented by controller firmware such as 5.15.x " +
			"and 6.2.x. Requires one of: `Site Settings Manager Modify` or `Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.StringAttribute{
				Description: "Group ID assigned by the controller. Use it (with site_id) as the import target " +
					"and as an ACL source_ids/destination_ids value when source_type/destination_type is 1.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID the group belongs to. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Group name. Must contain 1 to 64 characters and be unique within the site.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Optional group description (1 to 256 characters). Do not store secrets here. " +
					"The controller stores this but does not return it in the group list read, so it is " +
					"preserved from configuration rather than refreshed from the API, and is not recoverable " +
					"on a bare import.",
				Optional: true,
			},
			"ip_list": schema.ListNestedAttribute{
				Description: "Ordered list of IP hosts/subnets the group matches. Provide at least one " +
					"entry; a /32 mask denotes a single host. The controller rejects an empty list.",
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"ip": schema.StringAttribute{
							Description: "IP address (the network/host address for the given mask).",
							Required:    true,
						},
						"mask": schema.Int32Attribute{
							Description: "CIDR prefix length, 1 to 32. Use 32 for a single host.",
							Required:    true,
						},
						"description": schema.StringAttribute{
							Description: "Optional per-entry description.",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

// decodeEnvelope reads the (re-readable) response body from an SDK call and
// decodes the standard Omada envelope leniently. This sidesteps the SDK's strict
// per-model decoders (which reject fields the controller returns that the SDK
// model does not know) and recovers the controller's errorCode/msg.
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

	// A transport/HTTP error (non-2xx) whose body decoded but carries no
	// errorCode (e.g. a reverse-proxy error page shaped as JSON) would otherwise
	// slip past hasError() and be treated as success; surface it instead.
	if callErr != nil && env.ErrorCode == nil {
		diags.AddError("Error "+action, "API call failed: "+callErr.Error())
		return omadaEnvelope{}, false
	}

	return env, true
}

// Create creates the resource and sets the initial Terraform state.
func (r *ipGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ipGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ProfilesAPI.CreateGroupProfile(ctx, r.omadacId, plan.SiteId.ValueString()).
		CreateGroupOpenApiVO(expandGroup(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating IP group")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating IP group", env.ErrorCode, env.Msg)
		return
	}

	// Prefer the id from the create result when present; otherwise locate the
	// newly-created group by name (names are unique within a site).
	if len(env.Result) > 0 {
		var cr createResult
		if err := json.Unmarshal(env.Result, &cr); err == nil && cr.Id != nil && *cr.Id != "" {
			plan.GroupId = types.StringValue(*cr.Id)
		}
	}
	// group_id is Computed, so it is Unknown (not Null) at create time. Treat both
	// as "not yet known" so the name-based fallback runs if the create response
	// ever omits the id (mirrors the omada_acl create-id recovery, fork v0.7.2).
	if (plan.GroupId.IsUnknown() || plan.GroupId.IsNull()) && !findGroupByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating IP group",
			"Create did not return an id and the group was not present in the site afterwards.",
		)
		return
	}

	if !readGroup(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state. The controller exposes no single-group
// GET, so Read fetches the per-type group list and selects the entry matching
// group_id.
func (r *ipGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ipGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readGroup(ctx, &resp.Diagnostics, r, &state) {
		return
	}

	// When the group is gone upstream, readGroup clears GroupId; drop it from
	// state so Terraform plans a re-create.
	if state.GroupId.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *ipGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ipGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ipGroupResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.GroupId = state.GroupId
	plan.SiteId = state.SiteId

	_, httpResp, callErr := r.client.ProfilesAPI.ModifyGroupProfile(ctx, r.omadacId, plan.SiteId.ValueString(), groupTypeIP, plan.GroupId.ValueString()).
		CreateGroupOpenApiVO(expandGroup(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating IP group")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating IP group", env.ErrorCode, env.Msg)
		return
	}

	if !readGroup(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ipGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ipGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ProfilesAPI.DeleteGroupProfile(ctx, r.omadacId, state.SiteId.ValueString(), state.GroupId.ValueString(), groupTypeIP).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "deleting IP group")
	if !ok {
		return
	}

	// A "group does not exist" code is a successful delete: the resource is
	// already gone, which is the desired end state. The controller also rejects
	// deleting a group still referenced by an ACL (-33717); surface that as an
	// error so the operator removes the reference first.
	if env.hasError() && env.ErrorCode != nil && !errGroupNotFound[*env.ErrorCode] {
		respondAPIError(&resp.Diagnostics, "deleting IP group", env.ErrorCode, env.Msg)
		return
	}
}

// ImportState imports an existing IP group. The import ID is
// `<site_id>/<group_id>`. The framework follows ImportState with a Read, which
// fully populates the remaining attributes from the controller.
func (r *ipGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<group_id>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), idParts[1])...)
}

// fetchGroupList fetches the IP-group list (group type 0) for the model's site
// and decodes it leniently. The per-type endpoint returns the groups as a bare
// JSON array under `result` (no {data} paging wrapper).
func fetchGroupList(ctx context.Context, diags *diag.Diagnostics, r *ipGroupResource, model *ipGroupResourceModel) []groupReadRow {
	_, httpResp, callErr := r.client.ProfilesAPI.GetGroupProfilesByType(ctx, r.omadacId, model.SiteId.ValueString(), groupTypeIP).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading IP group")
	if !ok {
		return nil
	}

	if env.hasError() {
		diags.AddError(
			"Error reading IP group",
			fmt.Sprintf("Controller rejected the list for site %s, error code %d: %s", model.SiteId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return nil
	}

	// An empty result (no groups yet) decodes to a nil slice, which is fine.
	if len(env.Result) == 0 {
		return nil
	}

	var rows []groupReadRow
	if err := json.Unmarshal(env.Result, &rows); err != nil {
		diags.AddError("Error reading IP group", "Could not decode group list: "+err.Error())
		return nil
	}

	return rows
}

// findGroupByName locates the group matching the model's name (used after create
// when the id is not returned). Sets model.GroupId on success; returns false if
// not found.
func findGroupByName(ctx context.Context, diags *diag.Diagnostics, r *ipGroupResource, model *ipGroupResourceModel) bool {
	data := fetchGroupList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range data {
		if data[i].Name == model.Name.ValueString() {
			model.GroupId = types.StringPointerValue(data[i].GroupId)
			return true
		}
	}

	return false
}

// readGroup selects the list entry matching the model's group_id and refreshes
// the model in place. When the group no longer exists, it clears model.GroupId
// so the caller can drop the resource from state.
func readGroup(ctx context.Context, diags *diag.Diagnostics, r *ipGroupResource, model *ipGroupResourceModel) bool {
	data := fetchGroupList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range data {
		row := &data[i]
		if row.GroupId != nil && *row.GroupId == model.GroupId.ValueString() {
			flattenGroupRead(model, row)
			return true
		}
	}

	// Not present in the list: the group is gone upstream.
	model.GroupId = types.StringNull()
	return true
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
