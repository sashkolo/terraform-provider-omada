package ssid

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
			},
			"guest_net_enable": schema.BoolAttribute{
				Description: "Treat this as a guest network (isolates clients). Defaults to `false`.",
				Optional:    true,
				Computed:    true,
			},
			"enable_11r": schema.BoolAttribute{
				Description: "Enable 802.11r fast roaming. Defaults to `false`.",
				Optional:    true,
				Computed:    true,
			},
			"hide_pwd": schema.BoolAttribute{
				Description: "Hide the PSK in the controller UI/API where supported. Defaults to `false`. " +
					"The provider still preserves the PSK in Terraform state regardless of this setting.",
				Optional: true,
				Computed: true,
			},
			"mlo_enable": schema.BoolAttribute{
				Description: "Enable Wi-Fi 7 multi-link operation. Defaults to `false`.",
				Optional:    true,
				Computed:    true,
			},
			"pmf_mode": schema.Int32Attribute{
				Description: "Protected Management Frames mode. `1` = Mandatory, `2` = Capable (default), " +
					"`3` = Disable.",
				Optional: true,
				Computed: true,
			},
			"device_type": schema.Int32Attribute{
				Description: "Target device bitfield. Bit 0 = EAP, bit 1 = Gateway. e.g. `3` targets both " +
					"(the controller default).",
				Optional: true,
				Computed: true,
			},
			"vlan_enable": schema.BoolAttribute{
				Description: "Tag client traffic to a VLAN. When `true`, `vlan_id` must be set. " +
					"Defaults to `false`.",
				Optional: true,
				Computed: true,
			},
			"vlan_id": schema.Int32Attribute{
				Description: "802.1Q VLAN tag for this SSID. Required and only sent when `vlan_enable` is `true`. " +
					"Must be in the range 1-4094.",
				Optional: true,
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
					},
					"psk_version": schema.Int32Attribute{
						Description: "PSK version. Defaults to `4` (WPA2/WPA3 Personal compatibility).",
						Optional:    true,
						Computed:    true,
					},
					"gik_rekey_psk_enable": schema.BoolAttribute{
						Description: "Enable group key rekey. Defaults to `false`.",
						Optional:    true,
						Computed:    true,
					},
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
func (r *ssidResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ssidResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WirelessNetworkAPI.CreateSsid(ctx, r.omadacId, plan.SiteId.ValueString(), plan.WlanGroupId.ValueString()).
		CreateSsidOpenApiVO(expandCreateSsid(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating SSID")
	if !ok {
		return
	}

	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating SSID", env.ErrorCode, env.Msg)
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
	if plan.SsidId.IsNull() && !awaitFindSsidByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating SSID",
			"Create did not return an id and the SSID was not present in the WLAN group afterwards.",
		)
		return
	}

	if !awaitReadSsid(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating SSID",
			"The SSID was created but could not be read back within the retry window.",
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data via the single-SSID
// detail endpoint. When the SSID is gone upstream, the SSID ID is cleared so
// Terraform drops the resource from state.
func (r *ssidResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ssidResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readSsid(ctx, &resp.Diagnostics, r, &state)

	if state.SsidId.IsNull() {
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

	_, httpResp, callErr := r.client.WirelessNetworkAPI.UpdateSsidBasicConfig(ctx, r.omadacId, plan.SiteId.ValueString(), plan.WlanGroupId.ValueString(), plan.SsidId.ValueString()).
		UpdateSsidBasicConfigOpenApiVO(expandUpdateSsid(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating SSID")
	if !ok {
		return
	}

	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating SSID", env.ErrorCode, env.Msg)
		return
	}

	if !readSsid(ctx, &resp.Diagnostics, r, &plan) {
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
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "deleting SSID")
	if !ok {
		return
	}

	// -1001 (invalid request parameters) is returned when the SSID no longer
	// exists on 5.15.x; that is the desired end state for a delete.
	if env.hasError() && env.ErrorCode != nil && *env.ErrorCode != errNotFound {
		respondAPIError(&resp.Diagnostics, "deleting SSID", env.ErrorCode, env.Msg)
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

// findSsidByName locates the SSID matching the model's name within its WLAN
// group (used after create when the id is not returned). Sets model.SsidId on
// success; returns false if not found.
func findSsidByName(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	_, httpResp, callErr := r.client.WirelessNetworkAPI.GetSsidList(ctx, r.omadacId, model.SiteId.ValueString(), model.WlanGroupId.ValueString()).
		Page(1).PageSize(1000).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading SSID")
	if !ok {
		return false
	}

	if env.hasError() {
		diags.AddError(
			"Error reading SSID",
			fmt.Sprintf("Controller rejected the SSID list for WLAN group %s, error code %d: %s", model.WlanGroupId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return false
	}

	var lr ssidListResult
	if err := json.Unmarshal(env.Result, &lr); err != nil {
		diags.AddError("Error reading SSID", "Could not decode SSID list: "+err.Error())
		return false
	}

	for i := range lr.Data {
		if lr.Data[i].Name != nil && *lr.Data[i].Name == model.Name.ValueString() {
			model.SsidId = types.StringPointerValue(lr.Data[i].SsidId)
			return true
		}
	}

	return false
}

// awaitFindSsidByName retries findSsidByName briefly. The controller's SSID
// list is eventually consistent right after create, so a single immediate read
// can miss an SSID that was just created.
func awaitFindSsidByName(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	return runWithBackoff(ctx, func() bool {
		return findSsidByName(ctx, diags, r, model)
	})
}

// readSsid fetches the SSID detail and refreshes the model in place. When the
// SSID no longer exists, it clears model.SsidId so the caller can drop the
// resource from state. The PSK is preserved from the prior model when the
// controller masks it on read.
func readSsid(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	_, httpResp, callErr := r.client.WirelessNetworkAPI.GetSsidDetail(ctx, r.omadacId, model.SiteId.ValueString(), model.WlanGroupId.ValueString(), model.SsidId.ValueString()).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading SSID")
	if !ok {
		return false
	}

	// -1001: the SSID is gone upstream. Drop it from state by clearing the id.
	if env.hasError() && env.ErrorCode != nil && *env.ErrorCode == errNotFound {
		model.SsidId = types.StringNull()
		return true
	}

	if env.hasError() {
		diags.AddError(
			"Error reading SSID",
			fmt.Sprintf("Controller rejected the detail read for SSID %s, error code %d: %s", model.SsidId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return false
	}

	var detail ssidDetailReadVO
	if err := json.Unmarshal(env.Result, &detail); err != nil {
		diags.AddError("Error reading SSID", "Could not decode SSID detail: "+err.Error())
		return false
	}

	flattenSsidRead(model, &detail)
	return true
}

// awaitReadSsid retries the detail read briefly after create so the just-created
// SSID is reflected despite propagation lag. Unlike readSsid it never clears the
// id (the SSID is known to exist; it just has not been readable yet). Returns
// false only if it never becomes readable.
func awaitReadSsid(ctx context.Context, diags *diag.Diagnostics, r *ssidResource, model *ssidResourceModel) bool {
	return runWithBackoff(ctx, func() bool {
		_, httpResp, callErr := r.client.WirelessNetworkAPI.GetSsidDetail(ctx, r.omadacId, model.SiteId.ValueString(), model.WlanGroupId.ValueString(), model.SsidId.ValueString()).Execute()
		env, ok := decodeEnvelope(httpResp, callErr, diags, "reading SSID")
		if !ok {
			return false
		}
		if env.hasError() {
			// Not readable yet (propagation lag); retry. A real error will
			// persist across attempts and surface after the budget is spent.
			return false
		}
		var detail ssidDetailReadVO
		if err := json.Unmarshal(env.Result, &detail); err != nil {
			diags.AddError("Error reading SSID", "Could not decode SSID detail: "+err.Error())
			return false
		}
		flattenSsidRead(model, &detail)
		return true
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
