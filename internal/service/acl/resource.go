package acl

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
func (r *aclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aclResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ACLAPI.CreateOsgAcl(ctx, r.omadacId, plan.SiteId.ValueString()).
		GatewayACLConfig(expandGatewayACL(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating ACL")
	if !ok {
		return
	}

	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating ACL", env.ErrorCode, env.Msg)
		return
	}

	// Prefer the id from the create result when present (aclId or generic id);
	// otherwise locate the newly-created ACL by description.
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
	if plan.AclId.IsNull() && !findAclByDescription(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating ACL",
			"Create did not return an id and the ACL was not present in the site afterwards.",
		)
		return
	}

	if !readAcl(ctx, &resp.Diagnostics, r, &plan) {
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

	readAcl(ctx, &resp.Diagnostics, r, &state)

	// When the ACL is gone upstream, readAcl clears AclId; leave resp.State unset
	// so Terraform drops it from state.
	if state.AclId.IsNull() {
		return
	}

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

	_, httpResp, callErr := r.client.ACLAPI.ModifyOsgAcl(ctx, r.omadacId, plan.SiteId.ValueString(), plan.AclId.ValueString()).
		GatewayACLConfig(expandGatewayACL(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating ACL")
	if !ok {
		return
	}

	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating ACL", env.ErrorCode, env.Msg)
		return
	}

	if !readAcl(ctx, &resp.Diagnostics, r, &plan) {
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
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "deleting ACL")
	if !ok {
		return
	}

	// A not-found on delete is a successful outcome (the resource is already
	// gone, which is the desired end state). Read clears AclId when an ACL is
	// removed upstream, so this only triggers when the ACL vanishes between
	// refresh and destroy. The exact not-found code is controller-dependent;
	// treat any non-zero code as an error for now and refine once confirmed
	// against the live controller.
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "deleting ACL", env.ErrorCode, env.Msg)
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
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading ACL")
	if !ok {
		return nil
	}

	if env.hasError() {
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

// findAclByDescription locates the ACL matching the model's description (used
// after create when the id is not returned). Sets model.AclId on success; returns
// false if not found.
func findAclByDescription(ctx context.Context, diags *diag.Diagnostics, r *aclResource, model *aclResourceModel) bool {
	data := fetchAclList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range data {
		if data[i].Description == model.Description.ValueString() {
			model.AclId = types.StringValue(data[i].Id)
			return true
		}
	}

	return false
}

// readAcl selects the list entry matching the model's acl_id and refreshes the
// model in place. When the ACL no longer exists, it clears model.AclId so the
// caller can drop the resource from state.
func readAcl(ctx context.Context, diags *diag.Diagnostics, r *aclResource, model *aclResourceModel) bool {
	data := fetchAclList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range data {
		row := &data[i]
		if row.Id == model.AclId.ValueString() {
			flattenAclRead(model, row)
			return true
		}
	}

	// Not present in the list: the ACL is gone upstream.
	model.AclId = types.StringNull()
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
