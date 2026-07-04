package ipportgroup

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// errGroupNotFound codes mark a missing group profile; Delete treats them as a
// successful outcome. Both codes mean "this group does not exist" in the Open
// API error table.
var errGroupNotFound = map[int32]bool{
	-33703: true,
	-33704: true,
}

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &ipPortGroupResource{}
	_ resource.ResourceWithConfigure   = &ipPortGroupResource{}
	_ resource.ResourceWithImportState = &ipPortGroupResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &ipPortGroupResource{}
}

// ipPortGroupResource is the resource implementation.
type ipPortGroupResource struct {
	ipPortGroupClient
}

// Configure adds the provider configured client to the resource.
func (r *ipPortGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *ipPortGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_port_group"
}

// Schema defines the schema for the resource.
func (r *ipPortGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Omada IP-Port group profile: a named set of IP hosts/subnets scoped to " +
			"a set of TCP/UDP ports, referenced by gateway ACL rules by group id " +
			"(source_type/destination_type = 2). It is the right object for service-level policy such as " +
			"\"allow a host to reach camera IPs only on the ONVIF/RTSP/HTTP ports\". Targets the Open API " +
			"v1 group surface (`/openapi/v1/.../profiles/groups`) on firmware such as 5.15.x/6.2.x. " +
			"Requires one of: `Site Settings Manager Modify` or `Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.StringAttribute{
				Description: "Group ID assigned by the controller. Use it (with site_id) as the import target " +
					"and as an ACL source_ids/destination_ids value when source_type/destination_type is 2.",
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
				Description: "Optional group description (1 to 256 characters). Do not store secrets here.",
				Optional:    true,
			},
			"ip_list": schema.ListNestedAttribute{
				Description: "Ordered list of IP hosts/subnets the ports are scoped to. Provide at least one " +
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
			"port_type": schema.Int32Attribute{
				Description: "Port match mode: `0` port list (use port_list, the default), `1` port mask " +
					"(use port_mask_list).",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"port_list": schema.ListAttribute{
				Description: "Ports the group matches when port_type is 0. Each entry is a single port " +
					"(\"554\") or an inclusive range (\"8000-8100\").",
				ElementType: types.StringType,
				Optional:    true,
			},
			"port_mask_list": schema.ListNestedAttribute{
				Description: "Port/mask pairs the group matches when port_type is 1.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"port": schema.Int32Attribute{
							Description: "Base port for the port/mask match.",
							Required:    true,
						},
						"mask": schema.StringAttribute{
							Description: "Port mask applied to the base port.",
							Required:    true,
						},
					},
				},
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

	return env, true
}

// Create creates the resource and sets the initial Terraform state.
func (r *ipPortGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ipPortGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ProfilesAPI.CreateGroupProfile(ctx, r.omadacId, plan.SiteId.ValueString()).
		CreateGroupOpenApiVO(expandGroup(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating IP-Port group")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating IP-Port group", env.ErrorCode, env.Msg)
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
	if plan.GroupId.IsNull() && !findGroupByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating IP-Port group",
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
func (r *ipPortGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ipPortGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readGroup(ctx, &resp.Diagnostics, r, &state) {
		return
	}

	if state.GroupId.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *ipPortGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ipPortGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ipPortGroupResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.GroupId = state.GroupId
	plan.SiteId = state.SiteId

	_, httpResp, callErr := r.client.ProfilesAPI.ModifyGroupProfile(ctx, r.omadacId, plan.SiteId.ValueString(), groupTypePort, plan.GroupId.ValueString()).
		CreateGroupOpenApiVO(expandGroup(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating IP-Port group")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating IP-Port group", env.ErrorCode, env.Msg)
		return
	}

	if !readGroup(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ipPortGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ipPortGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ProfilesAPI.DeleteGroupProfile(ctx, r.omadacId, state.SiteId.ValueString(), state.GroupId.ValueString(), groupTypePort).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "deleting IP-Port group")
	if !ok {
		return
	}

	// A "group does not exist" code is a successful delete. The controller also
	// rejects deleting a group still referenced by an ACL (-33718); surface that
	// as an error so the operator removes the reference first.
	if env.hasError() && env.ErrorCode != nil && !errGroupNotFound[*env.ErrorCode] {
		respondAPIError(&resp.Diagnostics, "deleting IP-Port group", env.ErrorCode, env.Msg)
		return
	}
}

// ImportState imports an existing IP-Port group. The import ID is
// `<site_id>/<group_id>`.
func (r *ipPortGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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

// fetchGroupList fetches the IP-Port-group list (group type 1) for the model's
// site and decodes it leniently. The per-type endpoint returns the groups as a
// bare JSON array under `result`.
func fetchGroupList(ctx context.Context, diags *diag.Diagnostics, r *ipPortGroupResource, model *ipPortGroupResourceModel) []groupReadRow {
	_, httpResp, callErr := r.client.ProfilesAPI.GetGroupProfilesByType(ctx, r.omadacId, model.SiteId.ValueString(), groupTypePort).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading IP-Port group")
	if !ok {
		return nil
	}

	if env.hasError() {
		diags.AddError(
			"Error reading IP-Port group",
			fmt.Sprintf("Controller rejected the list for site %s, error code %d: %s", model.SiteId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return nil
	}

	if len(env.Result) == 0 {
		return nil
	}

	var rows []groupReadRow
	if err := json.Unmarshal(env.Result, &rows); err != nil {
		diags.AddError("Error reading IP-Port group", "Could not decode group list: "+err.Error())
		return nil
	}

	return rows
}

// findGroupByName locates the group matching the model's name (used after create
// when the id is not returned). Sets model.GroupId on success.
func findGroupByName(ctx context.Context, diags *diag.Diagnostics, r *ipPortGroupResource, model *ipPortGroupResourceModel) bool {
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
func readGroup(ctx context.Context, diags *diag.Diagnostics, r *ipPortGroupResource, model *ipPortGroupResourceModel) bool {
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
