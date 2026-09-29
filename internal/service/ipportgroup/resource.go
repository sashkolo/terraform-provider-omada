package ipportgroup

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
				Description: "Optional group description (1 to 256 characters). Do not store secrets here. " +
					"The controller stores this but does not return it in the group list read, so it is " +
					"preserved from configuration rather than refreshed from the API, and is not recoverable " +
					"on a bare import.",
				Optional: true,
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
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "creating IP-Port group")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "creating IP-Port group", env.ErrorCode, env.Msg)
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
	if (plan.GroupId.IsUnknown() || plan.GroupId.IsNull()) && !awaitFindGroupByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating IP-Port group",
			"Create did not return an id and the group was not present in the site afterwards.",
		)
		return
	}

	if !awaitReadGroup(ctx, &resp.Diagnostics, r, &plan) {
		// The group exists on the controller: keep it in state (tainted) with
		// the id the POST returned rather than a null id, which an ACL would
		// reference as a null element (homelab #514).
		resp.Diagnostics.Append(tfstate.SaveCreated(ctx, req.Plan, &resp.State, "group_id", plan.GroupId.ValueString())...)
		resp.Diagnostics.AddError(
			"Error creating IP-Port group",
			fmt.Sprintf("IP-Port group %s was created but could not be read back within the retry window. It is "+
				"kept in state as tainted, so the next apply replaces it.", plan.GroupId.ValueString()),
		)
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

	row, found := findGroupConfirmed(ctx, &resp.Diagnostics, r, &state)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		// Gone upstream (confirmed by repeated misses, as the list is
		// eventually consistent): drop it so Terraform plans a re-create.
		resp.State.RemoveResource(ctx)
		return
	}
	flattenGroupRead(&state, row)

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
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "updating IP-Port group")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "updating IP-Port group", env.ErrorCode, env.Msg)
		return
	}

	if !awaitReadGroup(ctx, &resp.Diagnostics, r, &plan) {
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
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "deleting IP-Port group")
	if !ok {
		return
	}

	// A "group does not exist" code is a successful delete. The controller also
	// rejects deleting a group still referenced by an ACL (-33718); surface that
	// as an error so the operator removes the reference first.
	if env.HasError() && !errGroupNotFound[env.Code()] {
		// Any other controller error is still success when a confirmed re-list
		// shows the group absent (homelab #514); a referenced group stays listed.
		var listDiags diag.Diagnostics
		if _, found := findGroupConfirmed(ctx, &listDiags, r, &state); !listDiags.HasError() && !found {
			return
		}
		envelope.AddAPIError(&resp.Diagnostics, "deleting IP-Port group", env.ErrorCode, env.Msg)
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
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading IP-Port group")
	if !ok {
		return nil
	}

	if env.HasError() {
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

// findGroupInList fetches the list and returns the entry matching the model's
// group_id (nil if not present). It does not mutate the model.
func findGroupInList(ctx context.Context, diags *diag.Diagnostics, r *ipPortGroupResource, model *ipPortGroupResourceModel) *groupReadRow {
	data := fetchGroupList(ctx, diags, r, model)
	if diags.HasError() {
		return nil
	}
	want := model.GroupId.ValueString()
	for i := range data {
		if data[i].GroupId != nil && *data[i].GroupId == want {
			return &data[i]
		}
	}
	return nil
}

// findGroupConfirmed looks the model's group_id up in the list. A miss is
// re-checked a few times before it is believed, because the list is eventually
// consistent; a found row is returned at once.
func findGroupConfirmed(ctx context.Context, diags *diag.Diagnostics, r *ipPortGroupResource, model *ipPortGroupResourceModel) (*groupReadRow, bool) {
	var row *groupReadRow
	found, last := retry.Until(ctx, goneConfirmations, retry.Interval, func(d *diag.Diagnostics) bool {
		row = findGroupInList(ctx, d, r, model)
		// Stop on an error too: a failing list is not evidence of absence.
		return row != nil || d.HasError()
	})
	diags.Append(last...)
	return row, found && row != nil
}

// goneConfirmations is how many list reads must miss a group before Read or
// Delete treats it as deleted.
const goneConfirmations = 3

// awaitReadGroup retries the list read until the group is present, refreshing
// the model in place. It never clears group_id: Create and Update already know
// the id, and a null id would reach any ACL that references the group (homelab
// #514). Only the last attempt's diagnostics are kept.
func awaitReadGroup(ctx context.Context, diags *diag.Diagnostics, r *ipPortGroupResource, model *ipPortGroupResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		if row := findGroupInList(ctx, d, r, model); row != nil {
			flattenGroupRead(model, row)
			return true
		}
		return false
	})
	diags.Append(last...)
	return ok
}

// awaitFindGroupByName retries the name-based list lookup until the group is
// present, setting model.GroupId. Used after create when the create response
// omitted the id; names are unique within a site.
func awaitFindGroupByName(ctx context.Context, diags *diag.Diagnostics, r *ipPortGroupResource, model *ipPortGroupResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		data := fetchGroupList(ctx, d, r, model)
		if d.HasError() {
			return false
		}
		for i := range data {
			if data[i].GroupId != nil && *data[i].GroupId != "" && data[i].Name == model.Name.ValueString() {
				model.GroupId = types.StringValue(*data[i].GroupId)
				return true
			}
		}
		return false
	})
	diags.Append(last...)
	return ok
}
