package firewallsetting

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
	_ resource.Resource                = &firewallSettingResource{}
	_ resource.ResourceWithConfigure   = &firewallSettingResource{}
	_ resource.ResourceWithImportState = &firewallSettingResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &firewallSettingResource{}
}

// firewallSettingResource is the resource implementation.
type firewallSettingResource struct {
	firewallSettingClient
}

// Configure adds the provider configured client to the resource.
func (r *firewallSettingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *firewallSettingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_setting"
}

// commonTimeouts describes the protocol conntrack timeouts managed by this
// resource (all in seconds, 1-2097151).
const commonTimeouts = "Each is in seconds and must be within the range 1-2097151."

// Schema defines the schema for the resource.
func (r *firewallSettingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the site-global Omada firewall settings (coarse blob): protocol " +
			"conntrack timeouts and a handful of hardening toggles. This is a singleton per " +
			"site: the object always exists, so import it (`<site_id>`) to adopt the live " +
			"settings before managing. Create/Update overwrite the whole object via Modify; " +
			"Delete is a no-op (the singleton is never removed, and Reset is never called). " +
			"Targets the Open API v1 firewall surface (controller firmware such as 5.15.x).",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				Description: "Site ID the settings belong to. The settings are a singleton per site; " +
					"use it as the import target. Changing this forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"broadcast_ping": schema.BoolAttribute{
				Description: "Whether the firewall responds to broadcast ping (hardening: leave `false`).",
				Required:    true,
			},
			"icmp": schema.Int32Attribute{
				Description: "ICMP conntrack timeout. " + commonTimeouts,
				Required:    true,
			},
			"other": schema.Int32Attribute{
				Description: "Other-protocol conntrack timeout. " + commonTimeouts,
				Required:    true,
			},
			"receive_redirects": schema.BoolAttribute{
				Description: "Whether the firewall accepts ICMP redirects (hardening: leave `false`).",
				Required:    true,
			},
			"send_redirects": schema.BoolAttribute{
				Description: "Whether the firewall sends ICMP redirects.",
				Required:    true,
			},
			"syn_cookies": schema.BoolAttribute{
				Description: "Whether SYN-flood cookie protection is enabled (hardening: leave `true`).",
				Required:    true,
			},
			"tcp_close":       int32Timeout("TCP CLOSE timeout."),
			"tcp_close_wait":  int32Timeout("TCP CLOSE_WAIT timeout."),
			"tcp_established": int32Timeout("TCP ESTABLISHED timeout."),
			"tcp_fin_wait":    int32Timeout("TCP FIN_WAIT timeout."),
			"tcp_last_ack":    int32Timeout("TCP LAST_ACK timeout."),
			"tcp_syn_receive": int32Timeout("TCP SYN_RECEIVED timeout."),
			"tcp_syn_sent":    int32Timeout("TCP SYN_SENT timeout."),
			"tcp_time_wait":   int32Timeout("TCP TIME_WAIT timeout."),
			"udp_other":       int32Timeout("UDP other timeout."),
			"udp_stream":      int32Timeout("UDP stream timeout."),
		},
	}
}

// int32Timeout returns a required Int32 schema attribute for a conntrack timeout.
func int32Timeout(desc string) schema.Int32Attribute {
	return schema.Int32Attribute{Description: desc + " " + commonTimeouts, Required: true}
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

// readFirewallSetting fetches the singleton and refreshes the model in place.
// It returns false only on a controller/transport error.
func readFirewallSetting(ctx context.Context, diags *diag.Diagnostics, r *firewallSettingResource, model *firewallSettingResourceModel) bool {
	_, httpResp, callErr := r.client.FirewallAPI.GetFirewallSetting(ctx, r.omadacId, model.SiteId.ValueString()).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading firewall setting")
	if !ok {
		return false
	}
	if env.hasError() {
		respondAPIError(diags, "reading firewall setting", env.ErrorCode, env.Msg)
		return false
	}
	var vo firewallReadVO
	if err := json.Unmarshal(env.Result, &vo); err != nil {
		diags.AddError("Error reading firewall setting", "Could not decode settings: "+err.Error())
		return false
	}
	flattenFirewallRead(model, &vo)
	return true
}

// Create applies the desired settings to the singleton (it always exists), then
// reads them back.
func (r *firewallSettingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallSettingResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.FirewallAPI.ModifyFirewallSetting(ctx, r.omadacId, plan.SiteId.ValueString()).
		FirewallSetting(expandFirewallSetting(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating firewall setting")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating firewall setting", env.ErrorCode, env.Msg)
		return
	}

	if !readFirewallSetting(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *firewallSettingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallSettingResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readFirewallSetting(ctx, &resp.Diagnostics, r, &state) {
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update overwrites the whole settings object, then reads it back.
func (r *firewallSettingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallSettingResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state firewallSettingResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteId = state.SiteId

	_, httpResp, callErr := r.client.FirewallAPI.ModifyFirewallSetting(ctx, r.omadacId, plan.SiteId.ValueString()).
		FirewallSetting(expandFirewallSetting(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating firewall setting")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating firewall setting", env.ErrorCode, env.Msg)
		return
	}

	if !readFirewallSetting(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete is a no-op: the firewall settings are a singleton that cannot be
// removed, and we never call Reset on destroy (that would silently change the
// controller). Removing the resource from state is the correct end state.
func (r *firewallSettingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState imports the live firewall settings. The import ID is `<site_id>`.
// The framework follows ImportState with a Read, which fully populates the
// remaining attributes from the controller.
func (r *firewallSettingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
