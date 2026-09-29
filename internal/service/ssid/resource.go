package ssid

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &ssidResource{}
	_ resource.ResourceWithConfigure   = &ssidResource{}
	_ resource.ResourceWithImportState = &ssidResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &ssidResource{}
}

// ssidResource is the resource implementation.
type ssidResource struct {
	ssidClient
}

// Configure adds the provider configured client to the resource.
func (r *ssidResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *ssidResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssid"
}

// Schema defines the schema for the resource.
func (r *ssidResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Omada WiFi SSID. An SSID always belongs to a WLAN group; create a " +
			"WLAN group that is not bound to any AP (omada_wlan_group) to stage SSIDs safely without " +
			"broadcasting. Targets the Open API v1 wireless-network surface implemented by controller " +
			"firmware such as 5.15.x. Requires one of: `Site Settings Manager Modify` or " +
			"`Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"ssid_id": schema.StringAttribute{
				Description: "SSID ID assigned by the controller. Use it (with site_id and wlan_group_id) as the import target.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID that owns the SSID. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"wlan_group_id": schema.StringAttribute{
				Description: "WLAN group the SSID belongs to (an omada_wlan_group or an existing group ID). " +
					"SSIDs cannot be moved between groups; changing this forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "SSID name broadcast to clients. Must contain 1 to 32 UTF-8 characters.",
				Required:    true,
			},
			"security": schema.Int32Attribute{
				Description: "Security mode. `0` = None (open), `2` = WPA-Enterprise, `3` = WPA-Personal (PSK), " +
					"`4` = PPSK without RADIUS, `5` = PPSK with RADIUS. Mode `3` requires a `psk_setting` block.",
				Required: true,
			},
			"band": schema.Int32Attribute{
				Description: "Radio band bitfield. Bit 0 = 2.4G, bit 1 = 5G, bit 2 = 6G. e.g. `7` enables " +
					"2.4G/5G/6G, `3` enables 2.4G/5G.",
				Required: true,
			},
			"broadcast": schema.BoolAttribute{
				Description: "Enable SSID broadcast. Defaults to `true`.",
				Optional:    true,
				Computed:    true,
				// Unset in config means "keep the live value": without this the value is
				// unknown on update and a hard-coded default is sent (homelab #515).
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"guest_net_enable": schema.BoolAttribute{
				Description: "Treat this as a guest network (isolates clients). Defaults to `false`.",
				Optional:    true,
				Computed:    true,
				// Unset in config means "keep the live value": without this the value is
				// unknown on update and a hard-coded default is sent (homelab #515).
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_11r": schema.BoolAttribute{
				Description: "Enable 802.11r fast roaming. Defaults to `false`.",
				Optional:    true,
				Computed:    true,
				// Unset in config means "keep the live value": without this the value is
				// unknown on update and a hard-coded default is sent (homelab #515).
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"hide_pwd": schema.BoolAttribute{
				Description: "Hide the PSK in the controller UI/API where supported. Defaults to `false`. " +
					"The provider still preserves the PSK in Terraform state regardless of this setting.",
				Optional: true,
				Computed: true,
				// Unset in config means "keep the live value": without this the value is
				// unknown on update and a hard-coded default is sent (homelab #515).
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"mlo_enable": schema.BoolAttribute{
				Description: "Enable Wi-Fi 7 multi-link operation. Defaults to `false`.",
				Optional:    true,
				Computed:    true,
				// Unset in config means "keep the live value": without this the value is
				// unknown on update and a hard-coded default is sent (homelab #515).
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"pmf_mode": schema.Int32Attribute{
				Description: "Protected Management Frames mode. `1` = Mandatory, `2` = Capable (default), " +
					"`3` = Disable.",
				Optional: true,
				Computed: true,
				// Unset in config means "keep the live value": without this the value is
				// unknown on update and a hard-coded default is sent (homelab #515).
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"device_type": schema.Int32Attribute{
				Description: "Target device bitfield. Bit 0 = EAP, bit 1 = Gateway. e.g. `3` targets both " +
					"(the controller default). The update endpoint cannot change it, so a change replaces the SSID.",
				Optional: true,
				Computed: true,
				// Unset in config means "keep the live value": without this the value is
				// unknown on update and a hard-coded default is sent (homelab #515).
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"vlan_enable": schema.BoolAttribute{
				Description: "Tag client traffic to a VLAN. When `true`, `vlan_id` must be set. " +
					"Defaults to `false`.",
				Optional: true,
				Computed: true,
				// Unset in config means "keep the live value": without this the value is
				// unknown on update and a hard-coded default is sent (homelab #515).
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"vlan_id": schema.Int32Attribute{
				Description: "802.1Q VLAN tag for this SSID. Required and only sent when `vlan_enable` is `true`. " +
					"Must be in the range 1-4094.",
				Optional: true,
				// Computed: the controller can report a tag while tagging is off.
				Computed: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"psk_setting": schema.SingleNestedAttribute{
				Description: "WPA-Personal key material. Required for security mode `3`; omit for open/enterprise.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"psk": schema.StringAttribute{
						Description: "WPA-Personal passphrase (8-63 chars). Sensitive: it round-trips through " +
							"remote state (expected) but is never printed in plan/log output. When the controller " +
							"masks the key on read, the provider preserves the prior-state value.",
						Optional:  true,
						Sensitive: true,
					},
					"psk_encryption": schema.Int32Attribute{
						Description: "PSK cipher. Defaults to `3` (the controller default on 5.15.x).",
						Optional:    true,
						Computed:    true,
						// Unset in config means "keep the live value": without this the value is
						// unknown on update and a hard-coded default is sent (homelab #515).
						PlanModifiers: []planmodifier.Int32{
							int32planmodifier.UseStateForUnknown(),
						},
					},
					"psk_version": schema.Int32Attribute{
						Description: "PSK version. Defaults to `4` (WPA2/WPA3 Personal compatibility).",
						Optional:    true,
						Computed:    true,
						// Unset in config means "keep the live value": without this the value is
						// unknown on update and a hard-coded default is sent (homelab #515).
						PlanModifiers: []planmodifier.Int32{
							int32planmodifier.UseStateForUnknown(),
						},
					},
					"gik_rekey_psk_enable": schema.BoolAttribute{
						Description: "Enable group key rekey. Defaults to `false`.",
						Optional:    true,
						Computed:    true,
						// Unset in config means "keep the live value": without this the value is
						// unknown on update and a hard-coded default is sent (homelab #515).
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
				},
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *ssidResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ssidResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WirelessNetworkAPI.CreateSsid(ctx, r.omadacId, plan.SiteId.ValueString(), plan.WlanGroupId.ValueString()).
		CreateSsidOpenApiVO(expandCreateSsid(plan)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "creating SSID")
	if !ok {
		return
	}

	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "creating SSID", env.ErrorCode, env.Msg)
		return
	}

	// Prefer the id from the create result when present; otherwise locate the
	// newly-created SSID by name within its WLAN group. The controller's SSID
	// state is eventually consistent, so both lookups retry briefly to
	// tolerate the post-create propagation lag.
	if len(env.Result) > 0 {
		var cr createResult
		if err := json.Unmarshal(env.Result, &cr); err == nil {
			if cr.SsidId != nil && *cr.SsidId != "" {
				plan.SsidId = types.StringValue(*cr.SsidId)
			} else if cr.Id != nil && *cr.Id != "" {
				plan.SsidId = types.StringValue(*cr.Id)
			}
		}
	}

	// ssid_id is Computed, so it is Unknown (not Null) at create time; testing
	// only IsNull skipped the name lookup and read back an empty id.
	if (plan.SsidId.IsUnknown() || plan.SsidId.IsNull()) && !awaitFindSsidByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating SSID",
			"Create did not return an id and the SSID was not present in the WLAN group afterwards.",
		)
		return
	}

	if !awaitReadSsid(ctx, &resp.Diagnostics, r, &plan) {
		// The SSID exists on the controller: keep it in state (tainted) rather
		// than orphaning it, so a re-apply replaces it instead of creating a
		// second SSID of the same name (homelab #514).
		resp.Diagnostics.Append(tfstate.SaveCreated(ctx, req.Plan, &resp.State, "ssid_id", plan.SsidId.ValueString())...)
		resp.Diagnostics.AddError(
			"Error creating SSID",
			fmt.Sprintf("SSID %s was created but could not be read back within the retry window. It is kept in "+
				"state as tainted, so the next apply replaces it.", plan.SsidId.ValueString()),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data via the single-SSID
// detail endpoint. A failed detail read is checked against the WLAN group's
// SSID list before the SSID is treated as deleted.
func (r *ssidResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ssidResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	found := readSsid(ctx, &resp.Diagnostics, r, &state)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		// Deleted outside Terraform. Setting nothing would keep the prior state,
		// which the framework pre-fills, so the SSID would silently stay
		// "managed" (homelab #514).
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *ssidResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ssidResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ssidResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SsidId = state.SsidId
	plan.SiteId = state.SiteId
	plan.WlanGroupId = state.WlanGroupId

	// Read-modify-write: settings the resource doesn't model are sent back with
	// their live values rather than left to the endpoint's defaults (homelab
	// #515). greEnable, oweEnable and prohibitWifiShare are not returned by the
	// detail read on 6.2.10, so they can't be carried; greEnable is still sent
	// as false because the endpoint rejects a body without it.
	live, refused := getSsidDetail(ctx, &resp.Diagnostics, r, &plan)
	if resp.Diagnostics.HasError() {
		return
	}
	if live == nil {
		resp.Diagnostics.AddError("Error updating SSID", refused)
		return
	}

	body := expandUpdateSsid(plan)
	body.AutoWanAccess = live.AutoWanAccess

	_, httpResp, callErr := r.client.WirelessNetworkAPI.UpdateSsidBasicConfig(ctx, r.omadacId, plan.SiteId.ValueString(), plan.WlanGroupId.ValueString(), plan.SsidId.ValueString()).
		UpdateSsidBasicConfigOpenApiVO(body).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "updating SSID")
	if !ok {
		return
	}

	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "updating SSID", env.ErrorCode, env.Msg)
		return
	}

	if !awaitReadSsid(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error updating SSID",
			fmt.Sprintf("SSID %s was updated but could not be read back within the retry window.", plan.SsidId.ValueString()),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ssidResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ssidResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WirelessNetworkAPI.DeleteSsid(ctx, r.omadacId, state.SiteId.ValueString(), state.WlanGroupId.ValueString(), state.SsidId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "deleting SSID")
	if !ok {
		return
	}

	// A rejected delete of an SSID that is already gone is the desired end
	// state. The controller answers a missing SSID with -1001, which is also
	// its generic "invalid request parameters", so a code alone once dropped a
	// live SSID from state; confirm against the list instead (homelab #514).
	if env.HasError() {
		var listDiags diag.Diagnostics
		if listed := ssidListedConfirmed(ctx, &listDiags, r, &state); !listDiags.HasError() && !listed {
			return
		}
		envelope.AddAPIError(&resp.Diagnostics, "deleting SSID", env.ErrorCode, env.Msg)
		return
	}
}

// ImportState imports an existing SSID. The import ID is
// `<site_id>/<wlan_group_id>/<ssid_id>`. The framework follows ImportState
// with a Read, which fully populates the remaining attributes from the
// controller.
func (r *ssidResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 3)
	if len(idParts) != 3 || idParts[0] == "" || idParts[1] == "" || idParts[2] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<wlan_group_id>/<ssid_id>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wlan_group_id"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ssid_id"), idParts[2])...)
}

// fetchSsidList fetches the SSID list of the model's WLAN group and decodes it
// leniently. The controller also rejects the list of a WLAN group that no
// longer exists (its SSIDs went with it); only when the group is missing from
// the site's group list is that rejection read as an empty list.
func fetchSsidList(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) []ssidListRow {
	_, httpResp, callErr := r.client.WirelessNetworkAPI.GetSsidList(ctx, r.omadacId, model.SiteId.ValueString(), model.WlanGroupId.ValueString()).
		Page(1).PageSize(1000).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading SSID")
	if !ok {
		return nil
	}

	if env.HasError() {
		var groupDiags diag.Diagnostics
		if !wlanGroupListed(ctx, &groupDiags, r, model) && !groupDiags.HasError() {
			return nil
		}
		diags.AddError(
			"Error reading SSID",
			fmt.Sprintf("Controller rejected the SSID list for WLAN group %s, error code %d: %s", model.WlanGroupId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return nil
	}

	var lr ssidListResult
	if err := json.Unmarshal(env.Result, &lr); err != nil {
		diags.AddError("Error reading SSID", "Could not decode SSID list: "+err.Error())
		return nil
	}

	return lr.Data
}

// wlanGroupListed reports whether the model's WLAN group is in the site's
// WLAN-group list (a bare array on 5.15.x, or the paged {data} shape).
func wlanGroupListed(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	_, httpResp, callErr := r.client.WirelessNetworkAPI.GetWlanGroupList(ctx, r.omadacId, model.SiteId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading WLAN group")
	if !ok {
		return false
	}

	if env.HasError() {
		envelope.AddAPIError(diags, "reading WLAN group", env.ErrorCode, env.Msg)
		return false
	}

	var rows []wlanGroupRow
	if err := json.Unmarshal(env.Result, &rows); err != nil {
		var paged struct {
			Data []wlanGroupRow `json:"data"`
		}
		if err := json.Unmarshal(env.Result, &paged); err != nil {
			diags.AddError("Error reading WLAN group", "Could not decode WLAN-group list: "+err.Error())
			return false
		}
		rows = paged.Data
	}

	for i := range rows {
		if rows[i].WlanId != nil && *rows[i].WlanId == model.WlanGroupId.ValueString() {
			return true
		}
	}
	return false
}

// ssidListed reports whether the model's ssid_id is in its WLAN group's list.
func ssidListed(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	rows := fetchSsidList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}
	for i := range rows {
		if rows[i].SsidId != nil && *rows[i].SsidId == model.SsidId.ValueString() {
			return true
		}
	}
	return false
}

// ssidListedConfirmed looks the model's ssid_id up in the list. A miss is
// re-checked a few times before it is believed, because the list is eventually
// consistent; a hit is returned at once. Callers must check diags first: a
// failing list is not evidence of absence.
func ssidListedConfirmed(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	var listed bool
	_, last := retry.Until(ctx, goneConfirmations, retry.Interval, func(d *diag.Diagnostics) bool {
		listed = ssidListed(ctx, d, r, model)
		return listed || d.HasError()
	})
	diags.Append(last...)
	return listed
}

// goneConfirmations is how many list reads must miss an SSID before Read or
// Delete treats it as deleted.
const goneConfirmations = 3

// findSsidByName locates the SSID matching the model's name within its WLAN
// group (used after create when the id is not returned). Sets model.SsidId on
// success; returns false if not found.
func findSsidByName(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	rows := fetchSsidList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range rows {
		if rows[i].Name != nil && *rows[i].Name == model.Name.ValueString() {
			model.SsidId = types.StringPointerValue(rows[i].SsidId)
			return true
		}
	}

	return false
}

// awaitFindSsidByName retries findSsidByName briefly. The controller's SSID
// list is eventually consistent right after create, so a single immediate read
// can miss an SSID that was just created. Only the last attempt's diagnostics
// are kept, so a transient error doesn't fail the apply.
func awaitFindSsidByName(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		return findSsidByName(ctx, d, r, model)
	})
	diags.Append(last...)
	return ok
}

// getSsidDetail fetches the SSID detail. It returns the decoded detail, or nil
// with the controller's refusal in refused when the controller answered but
// did not return the SSID (an error code, or a result without an ssidId).
// Transport and decode failures are added to diags.
func getSsidDetail(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) (detail *ssidDetailReadVO, refused string) {
	_, httpResp, callErr := r.client.WirelessNetworkAPI.GetSsidDetail(ctx, r.omadacId, model.SiteId.ValueString(), model.WlanGroupId.ValueString(), model.SsidId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading SSID")
	if !ok {
		return nil, ""
	}

	if env.HasError() {
		return nil, fmt.Sprintf("Controller rejected the detail read for SSID %s, error code %d: %s.", model.SsidId.ValueString(), *env.ErrorCode, env.Msg)
	}

	var d ssidDetailReadVO
	if len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, &d); err != nil {
			diags.AddError("Error reading SSID", "Could not decode SSID detail: "+err.Error())
			return nil, ""
		}
	}
	if d.SsidId == nil {
		return nil, fmt.Sprintf("Controller returned no detail for SSID %s.", model.SsidId.ValueString())
	}
	return &d, ""
}

// readSsid fetches the SSID detail and refreshes the model in place. The PSK
// is preserved from the prior model when the controller masks it on read.
//
// It returns false with no error only when the SSID is confirmed gone. The
// controller answers a missing SSID with -1001, which is also its generic
// "invalid request parameters", so a refused detail read is never believed on
// its own: the WLAN group's SSID list must also miss the SSID. Believing the
// code once risked dropping a live SSID from state (homelab #514).
func readSsid(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	detail, refused := getSsidDetail(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}
	if detail != nil {
		flattenSsidRead(model, detail)
		return true
	}

	var listDiags diag.Diagnostics
	listed := ssidListedConfirmed(ctx, &listDiags, r, model)
	if listDiags.HasError() {
		diags.AddError("Error reading SSID", refused)
		diags.Append(listDiags...)
		return false
	}
	if listed {
		diags.AddError("Error reading SSID", fmt.Sprintf("%s The SSID is still listed in WLAN group %s, so it is not "+
			"treated as deleted.", refused, model.WlanGroupId.ValueString()))
	}
	return false
}

// awaitReadSsid retries the detail read briefly after create or update so the
// SSID is reflected despite propagation lag. It never treats the SSID as gone:
// the caller knows it exists. Only the last attempt's diagnostics are kept, so
// a transient error doesn't fail the apply.
func awaitReadSsid(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		detail, refused := getSsidDetail(ctx, d, r, model)
		if d.HasError() {
			return false
		}
		if detail == nil {
			d.AddError("Error reading SSID", refused)
			return false
		}
		flattenSsidRead(model, detail)
		return true
	})
	diags.Append(last...)
	return ok
}
