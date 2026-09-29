package acl

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &aclResource{}
	_ resource.ResourceWithConfigure   = &aclResource{}
	_ resource.ResourceWithImportState = &aclResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &aclResource{}
}

// aclResource is the resource implementation.
type aclResource struct {
	aclClient
}

// Configure adds the provider configured client to the resource.
func (r *aclResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*client.Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Meta, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = data.Client
	r.omadacId = data.OmadacId
}

// Metadata returns the resource type name.
func (r *aclResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl"
}

// Schema defines the schema for the resource.
func (r *aclResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Omada gateway (OSG) ACL rule — the core building block of " +
			"inter-VLAN segmentation on the Omada gateway. Targets the Open API v1 gateway ACL " +
			"surface implemented by controller firmware such as 5.15.x. The rule `index` (order) " +
			"is read-only: the controller owns ACL ordering via a site-global ModifyAclIndex call " +
			"that this resource does not steer. Requires one of: `Site Settings Manager Modify`, " +
			"`Network Config Page Modify`, or `Site Device Manager Modify`.",
		Attributes: map[string]schema.Attribute{
			"acl_id": schema.StringAttribute{
				Description: "ACL ID assigned by the controller. Use it (with site_id) as the import target.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID to create the ACL in. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"index": schema.Int32Attribute{
				Description: "Rule order assigned by the controller (read-only). Omada reorders ACLs via a " +
					"site-global, per-device-type call carrying the full ordered id map, so a single ACL " +
					"resource cannot own its absolute index without fighting out-of-band changes. The " +
					"resource observes the controller-assigned index and never sends it.",
				Computed: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Description: "ACL rule description (also the fallback identity used to locate the rule " +
					"after create when the controller omits the id). Must contain 1 to 512 characters.",
				Required: true,
			},
			"source_type": schema.Int32Attribute{
				Description: "Source type. `0` = network; `1` = IP Group; `2` = IP-Port Group; `4` = SSID; " +
					"`6` = IPv6 Group; `7` = IPv6-Port Group; `8` = Country; `9` = Country Group; " +
					"`11` = !Network; `12` = !IP Group; `13` = !IP-Port Group; `14` = !IPv6 Group; " +
					"`15` = !IPv6-Port Group.",
				Required: true,
			},
			"source_ids": schema.ListAttribute{
				Description: "Source IDs, whose meaning depends on source_type (e.g. LAN network IDs when " +
					"source_type is network).",
				ElementType: types.StringType,
				Required:    true,
			},
			"destination_type": schema.Int32Attribute{
				Description: "Destination type. `0` = network; `1` = IP Group; `2` = IP-Port Group; " +
					"`6` = IPv6 Group; `7` = IPv6-Port Group; `10` = Domain Group; `11` = !Network; " +
					"`12` = !IP Group; `13` = !IP-Port Group; `14` = !IPv6 Group; `15` = !IPv6-Port Group.",
				Required: true,
			},
			"destination_ids": schema.ListAttribute{
				Description: "Destination IDs, whose meaning depends on destination_type. Optional on write " +
					"(computed on read): the controller populates this for types that resolve ids.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"policy": schema.Int32Attribute{
				Description: "Rule action. `0` = drop; `1` = allow.",
				Required:    true,
			},
			"protocols": schema.ListAttribute{
				Description: "Protocol numbers; refer to section 5.5 of the Omada Open API Access Guide.",
				ElementType: types.Int32Type,
				Required:    true,
			},
			"state_mode": schema.Int32Attribute{
				Description: "Conntrack state match mode. `0` = auto; `1` = manual (use the states block).",
				Required:    true,
			},
			"status": schema.BoolAttribute{
				Description: "Whether the rule is enabled.",
				Required:    true,
			},
			"syslog": schema.BoolAttribute{
				Description: "Whether to log matches to remote syslog.",
				Required:    true,
			},
			"direction": schema.SingleNestedAttribute{
				Description: "Gateway direction selector. At least one direction flag must be set; " +
					"lan_to_lan conflicts with the other directions.",
				Required: true,
				Attributes: map[string]schema.Attribute{
					"lan_to_lan": schema.BoolAttribute{
						Description: "Match LAN-to-LAN direction. Conflicts with the other directions.",
						Optional:    true,
					},
					"lan_to_wan": schema.BoolAttribute{
						Description: "Match LAN-to-WAN direction.",
						Optional:    true,
					},
					"vpn_in_ids": schema.ListAttribute{
						Description: "Selected VPN IDs (inbound) when matching VPN-sourced traffic.",
						ElementType: types.StringType,
						Optional:    true,
					},
					"wan_in_ids": schema.ListAttribute{
						Description: "Selected WAN port IDs (inbound) when matching WAN-sourced traffic.",
						ElementType: types.StringType,
						Optional:    true,
					},
				},
			},
			"states": schema.SingleNestedAttribute{
				Description: "Conntrack states to match. Only meaningful when state_mode is manual (1).",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"established": schema.BoolAttribute{
						Description: "Match the established state.",
						Optional:    true,
					},
					"invalid": schema.BoolAttribute{
						Description: "Match the invalid state.",
						Optional:    true,
					},
					"related": schema.BoolAttribute{
						Description: "Match the related state.",
						Optional:    true,
					},
					"state_new": schema.BoolAttribute{
						Description: "Match the new state.",
						Optional:    true,
					},
				},
			},
			"time_range_id": schema.StringAttribute{
				Description: "Gateway ACL time range ID. Omit for an always-active rule.",
				Optional:    true,
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *aclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aclResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	checkReferencedIDs(plan, &resp.Diagnostics, "creating ACL")
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ACLAPI.CreateOsgAcl(ctx, r.omadacId, plan.SiteId.ValueString()).
		GatewayACLConfig(expandGatewayACL(plan)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "creating ACL")
	if !ok {
		return
	}

	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "creating ACL", env.ErrorCode, env.Msg)
		return
	}

	// Prefer the id from the create result when present (aclId or generic id);
	// otherwise fall back to a description-based list lookup below. The
	// controller's ACL state is eventually consistent, so the lookup retries
	// briefly to tolerate the post-create propagation lag.
	if len(env.Result) > 0 {
		var cr createResult
		if err := json.Unmarshal(env.Result, &cr); err == nil {
			if cr.AclId != nil && *cr.AclId != "" {
				plan.AclId = types.StringValue(*cr.AclId)
			} else if cr.Id != nil && *cr.Id != "" {
				plan.AclId = types.StringValue(*cr.Id)
			}
		}
	}

	// acl_id is Computed, so it is Unknown (not Null) at create time. Treat both
	// as "not yet known" so the description-based list lookup runs when the create
	// response omitted the id (the live controller returns no result on create).
	if (plan.AclId.IsUnknown() || plan.AclId.IsNull()) && !awaitFindAclByDescription(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating ACL",
			"Create did not return an id and the ACL was not present in the site afterwards.",
		)
		return
	}

	if !awaitReadAcl(ctx, &resp.Diagnostics, r, &plan) {
		// The rule exists on the controller: keep it in state (tainted) rather
		// than orphaning it, so a re-apply replaces it instead of failing on a
		// duplicate description.
		resp.Diagnostics.Append(tfstate.SaveCreated(ctx, req.Plan, &resp.State, "acl_id", plan.AclId.ValueString())...)
		resp.Diagnostics.AddError(
			"Error creating ACL",
			fmt.Sprintf("ACL %s was created but could not be read back within the retry window. It is kept in "+
				"state as tainted, so the next apply replaces it.", plan.AclId.ValueString()),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data. The controller exposes
// no single-ACL GET, so Read fetches the paged gateway ACL list and selects the
// entry matching acl_id.
func (r *aclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aclResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	row, found := findAclConfirmed(ctx, &resp.Diagnostics, r, &state)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		// Deleted outside Terraform. Setting nothing would keep the prior state,
		// which the framework pre-fills, so the rule would silently stay
		// "managed".
		resp.State.RemoveResource(ctx)
		return
	}
	flattenAclRead(&state, row)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *aclResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan aclResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state aclResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.AclId = state.AclId
	plan.SiteId = state.SiteId

	checkReferencedIDs(plan, &resp.Diagnostics, "updating ACL")
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ACLAPI.ModifyOsgAcl(ctx, r.omadacId, plan.SiteId.ValueString(), plan.AclId.ValueString()).
		GatewayACLConfig(expandGatewayACL(plan)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "updating ACL")
	if !ok {
		return
	}

	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "updating ACL", env.ErrorCode, env.Msg)
		return
	}

	if !awaitReadAcl(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *aclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aclResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ACLAPI.DeleteAcl(ctx, r.omadacId, state.SiteId.ValueString(), state.AclId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "deleting ACL")
	if !ok {
		return
	}

	// A rejected delete of a rule that is already gone is the desired end
	// state. The controller's not-found code isn't documented for this
	// endpoint, so confirm against the list instead of trusting a code.
	if env.HasError() {
		var listDiags diag.Diagnostics
		if _, found := findAclConfirmed(ctx, &listDiags, r, &state); !listDiags.HasError() && !found {
			return
		}
		envelope.AddAPIError(&resp.Diagnostics, "deleting ACL", env.ErrorCode, env.Msg)
		return
	}
}

// ImportState imports an existing gateway ACL. The import ID is
// `<site_id>/<acl_id>`. The framework follows ImportState with a Read, which
// fully populates the remaining attributes from the controller.
func (r *aclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<acl_id>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("acl_id"), idParts[1])...)
}

// fetchAclList fetches the paged gateway ACL list for the model's site and
// decodes it leniently.
func fetchAclList(ctx context.Context, diags *diag.Diagnostics, r *aclResource, model *aclResourceModel) []aclReadRow {
	_, httpResp, callErr := r.client.ACLAPI.GetOsgAclList(ctx, r.omadacId, model.SiteId.ValueString()).
		Page(1).PageSize(1000).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading ACL")
	if !ok {
		return nil
	}

	if env.HasError() {
		diags.AddError(
			"Error reading ACL",
			fmt.Sprintf("Controller rejected the list for site %s, error code %d: %s", model.SiteId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return nil
	}

	var lr listResult
	if err := json.Unmarshal(env.Result, &lr); err != nil {
		diags.AddError("Error reading ACL", "Could not decode ACL list: "+err.Error())
		return nil
	}

	return lr.Data
}

// findAclInList fetches the paged gateway ACL list and returns the entry matching
// the model's acl_id (nil if not present). It does not mutate the model.
func findAclInList(ctx context.Context, diags *diag.Diagnostics, r *aclResource, model *aclResourceModel) *aclReadRow {
	data := fetchAclList(ctx, diags, r, model)
	if diags.HasError() {
		return nil
	}
	want := model.AclId.ValueString()
	for i := range data {
		if data[i].Id == want {
			return &data[i]
		}
	}
	return nil
}

// findAclConfirmed looks the model's acl_id up in the list. A miss is
// re-checked a few times before it is believed, because the list is eventually
// consistent; a found row is returned at once.
func findAclConfirmed(ctx context.Context, diags *diag.Diagnostics, r *aclResource, model *aclResourceModel) (*aclReadRow, bool) {
	var row *aclReadRow
	found, last := retry.Until(ctx, goneConfirmations, retry.Interval, func(d *diag.Diagnostics) bool {
		row = findAclInList(ctx, d, r, model)
		// Stop on an error too: a failing list is not evidence of absence.
		return row != nil || d.HasError()
	})
	diags.Append(last...)
	return row, found && row != nil
}

// goneConfirmations is how many list reads must miss a rule before Read or
// Delete treats it as deleted.
const goneConfirmations = 3

// awaitReadAcl retries the list read until the ACL is present, refreshing the
// model in place. The controller's ACL state is eventually consistent, so a
// freshly created/modified ACL may take a moment to appear in the list. It never
// clears acl_id: the caller (Create/Update) already knows the id. Only the last
// attempt's diagnostics are kept, so a transient error doesn't fail the apply.
func awaitReadAcl(ctx context.Context, diags *diag.Diagnostics, r *aclResource, model *aclResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		if row := findAclInList(ctx, d, r, model); row != nil {
			flattenAclRead(model, row)
			return true
		}
		return false
	})
	diags.Append(last...)
	return ok
}

// awaitFindAclByDescription retries the description-based list lookup until the
// ACL is present, setting model.AclId. Used after create when the create
// response omitted the id. Descriptions are unique per site (the controller
// rejects a duplicate with -33006), so a match is the rule just created.
func awaitFindAclByDescription(ctx context.Context, diags *diag.Diagnostics, r *aclResource, model *aclResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		data := fetchAclList(ctx, d, r, model)
		if d.HasError() {
			return false
		}
		for i := range data {
			if data[i].Description == model.Description.ValueString() {
				model.AclId = types.StringValue(data[i].Id)
				return true
			}
		}
		return false
	})
	diags.Append(last...)
	return ok
}
