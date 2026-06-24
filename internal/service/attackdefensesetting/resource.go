package attackdefensesetting

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
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &attackDefenseSettingResource{}
	_ resource.ResourceWithConfigure   = &attackDefenseSettingResource{}
	_ resource.ResourceWithImportState = &attackDefenseSettingResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &attackDefenseSettingResource{}
}

// attackDefenseSettingResource is the resource implementation.
type attackDefenseSettingResource struct {
	attackDefenseSettingClient
}

// Configure adds the provider configured client to the resource.
func (r *attackDefenseSettingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *attackDefenseSettingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_attack_defense_setting"
}

// Schema defines the schema for the resource.
func (r *attackDefenseSettingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the site-global Omada attack-defense settings (coarse blob): DoS, " +
			"flood, scan, and ping protections. This is a singleton per site: the object always " +
			"exists, so import it (`<site_id>`) to adopt the live settings before managing. " +
			"Create/Update overwrite the whole object via Modify; Delete is a no-op (the singleton " +
			"is never removed, and Reset is never called). Targets the Open API v1 attack-defense " +
			"surface (controller firmware such as 5.15.x).",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				Description: "Site ID the settings belong to. The settings are a singleton per site; " +
					"use it as the import target. Changing this forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"icmp_conn_enable":              requiredBool("Whether ICMP connection-rate flood protection is enabled."),
			"icmp_conn_limit":               optionalInt32("ICMP connection-rate flood threshold (per second)."),
			"icmp_src_enable":               requiredBool("Whether ICMP source-rate flood protection is enabled."),
			"icmp_src_limit":                optionalInt32("ICMP source-rate flood threshold (per second)."),
			"icmp_timestamp_request_reject": optionalBool("Whether to reject ICMP timestamp requests."),
			"large_ping_enable":             requiredBool("Whether large-ping protection is enabled."),
			"large_ping_threshold":          optionalInt32("Large-ping size threshold (bytes)."),
			"ping_death_enable":             requiredBool("Whether ping-of-death protection is enabled."),
			"ping_wan_enable":               requiredBool("Whether the WAN interface responds to ping."),
			"specified_option_enable":       requiredBool("Whether IP-option attack protection is enabled."),
			"specified_option": schema.SingleNestedAttribute{
				Description: "Per-IP-option attack-defense toggles. Meaningful when specified_option_enable is true.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"no_operation_enable":    optionalBool("Reject packets with the IP No Operation option."),
					"record_route_enable":    optionalBool("Reject packets with the IP Record Route option."),
					"security_option_enable": optionalBool("Reject packets with the IP Security option."),
					"stream_enable":          optionalBool("Reject packets with the IP Stream option."),
					"timestamp_enable":       optionalBool("Reject packets with the IP Timestamp option."),
				},
			},
			"tcp_conn_enable":        requiredBool("Whether TCP connection-rate flood protection is enabled."),
			"tcp_conn_limit":         optionalInt32("TCP connection-rate flood threshold (per second)."),
			"tcp_fin_no_ack_enable":  requiredBool("Whether TCP FIN-no-ACK scan protection is enabled."),
			"tcp_scan_enable":        requiredBool("Whether TCP scan protection is enabled."),
			"tcp_scan_reject":        optionalBool("Whether to reject detected TCP scans."),
			"tcp_src_enable":         requiredBool("Whether TCP source-rate flood protection is enabled."),
			"tcp_src_limit":          optionalInt32("TCP source-rate flood threshold (per second)."),
			"tcp_syn_fin_enable":     requiredBool("Whether TCP SYN-FIN scan protection is enabled."),
			"udp_conn_enable":        requiredBool("Whether UDP connection-rate flood protection is enabled."),
			"udp_conn_limit":         optionalInt32("UDP connection-rate flood threshold (per second)."),
			"udp_src_enable":         requiredBool("Whether UDP source-rate flood protection is enabled."),
			"udp_src_limit":          optionalInt32("UDP source-rate flood threshold (per second)."),
			"win_nuke_attack_enable": requiredBool("Whether WinNuke attack protection is enabled."),
		},
	}
}

func requiredBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{Description: desc, Required: true}
}
func optionalBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{Description: desc, Optional: true}
}
func optionalInt32(desc string) schema.Int32Attribute {
	return schema.Int32Attribute{Description: desc, Optional: true}
}

// decodeEnvelope reads the (re-readable) response body from an SDK call and
// decodes the standard Omada envelope leniently. This sidesteps the SDK's strict
// per-model decoders (DisallowUnknownFields) and recovers the controller's
// errorCode/msg. See envelope.go for why this is mandatory on 5.15.x.
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
		diags.AddError("Error "+action, "Could not decode response: "+jsonErr.Error())
		return omadaEnvelope{}, false
	}

	return env, true
}

// readAttackDefenseSetting fetches the singleton and refreshes the model in
// place. It returns false only on a controller/transport error.
func readAttackDefenseSetting(ctx context.Context, diags *diag.Diagnostics, r *attackDefenseSettingResource, model *attackDefenseSettingResourceModel) bool {
	_, httpResp, callErr := r.client.AttackDefenseAPI.GetAttackDefenseSetting(ctx, r.omadacId, model.SiteId.ValueString()).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading attack-defense setting")
	if !ok {
		return false
	}
	if env.hasError() {
		respondAPIError(diags, "reading attack-defense setting", env.ErrorCode, env.Msg)
		return false
	}
	var vo attackDefenseReadVO
	if err := json.Unmarshal(env.Result, &vo); err != nil {
		diags.AddError("Error reading attack-defense setting", "Could not decode settings: "+err.Error())
		return false
	}
	flattenAttackDefenseRead(model, &vo)
	return true
}

// Create applies the desired settings to the singleton (it always exists), then
// reads them back.
func (r *attackDefenseSettingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan attackDefenseSettingResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.AttackDefenseAPI.ModifyAttackDefenseSetting(ctx, r.omadacId, plan.SiteId.ValueString()).
		AttackDefenseSetting(expandAttackDefenseSetting(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating attack-defense setting")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating attack-defense setting", env.ErrorCode, env.Msg)
		return
	}

	if !readAttackDefenseSetting(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *attackDefenseSettingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state attackDefenseSettingResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readAttackDefenseSetting(ctx, &resp.Diagnostics, r, &state) {
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update overwrites the whole settings object, then reads it back.
func (r *attackDefenseSettingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan attackDefenseSettingResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state attackDefenseSettingResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteId = state.SiteId

	_, httpResp, callErr := r.client.AttackDefenseAPI.ModifyAttackDefenseSetting(ctx, r.omadacId, plan.SiteId.ValueString()).
		AttackDefenseSetting(expandAttackDefenseSetting(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating attack-defense setting")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating attack-defense setting", env.ErrorCode, env.Msg)
		return
	}

	if !readAttackDefenseSetting(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete is a no-op: the attack-defense settings are a singleton that cannot be
// removed, and we never call Reset on destroy (that would silently change the
// controller). Removing the resource from state is the correct end state.
func (r *attackDefenseSettingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState imports the live attack-defense settings. The import ID is
// `<site_id>`. The framework follows ImportState with a Read, which fully
// populates the remaining attributes from the controller.
func (r *attackDefenseSettingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" || strings.Contains(req.ID, "/") {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>` (the settings are a singleton per site), got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), req.ID)...)
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
