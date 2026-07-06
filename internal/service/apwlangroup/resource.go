package apwlangroup

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

// errNotFound is the Open API's generic "invalid request parameters" errorCode,
// which the controller returns when a path-addressed object does not exist — the
// same code the SSID detail read uses to detect a deleted SSID. For the AP
// overview read it signals a forgotten/removed AP so the resource is dropped
// from state rather than failing every plan. (If a future firmware reports a
// different code for a missing AP, the fallback is the prior hard-error
// behavior, so this is safe to assume until the live proof confirms it.)
const errNotFound int32 = -1001

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &apWlanGroupResource{}
	_ resource.ResourceWithConfigure   = &apWlanGroupResource{}
	_ resource.ResourceWithImportState = &apWlanGroupResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &apWlanGroupResource{}
}

// apWlanGroupResource is the resource implementation.
type apWlanGroupResource struct {
	apWlanGroupClient
}

// Configure adds the provider configured client to the resource.
func (r *apWlanGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *apWlanGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ap_wlan_group"
}

// Schema defines the schema for the resource.
func (r *apWlanGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages which single WLAN group a managed Omada access point (EAP) broadcasts. " +
			"The Open API models the AP↔WLAN-group relationship as 1:1 — every AP belongs to exactly " +
			"one group — and exposes only a \"switch this AP to a different group\" write, never an " +
			"unbind. This resource therefore behaves like a singleton keyed by (site_id, ap_mac): " +
			"Create/Update switch the AP to the target group (skipping the call when it is already on " +
			"it, which the controller rejects) and Delete is a no-op. Targets the Open API v1 AP " +
			"surface (`GET /aps/{apMac}` read, `PATCH /aps/{apMac}/wlan-group` write). Switching an " +
			"AP's group changes which SSIDs it broadcasts and can disrupt clients, so applies are " +
			"operator-gated. Requires `Site Device Manager Modify`.",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				Description: "Site ID the AP belongs to. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ap_mac": schema.StringAttribute{
				Description: "MAC address of the access point (e.g. `A4-2B-B0-11-22-33`). Changing this " +
					"forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"wlan_group_id": schema.StringAttribute{
				Description: "ID of the omada_wlan_group the AP should broadcast. Changing this switches " +
					"the AP to that group (and thus its SSIDs). Must reference a WLAN group in the same site.",
				Required: true,
			},
			"ap_name": schema.StringAttribute{
				Description: "Read-only AP name/alias as reported by the controller.",
				Computed:    true,
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
			// The controller likely returned a non-JSON body (HTML error page,
			// empty body) alongside an HTTP/transport error; surface it so the
			// real status code is not lost behind a generic parse error.
			msg += fmt.Sprintf(" (original error: %s)", callErr.Error())
		}
		diags.AddError("Error "+action, msg)
		return omadaEnvelope{}, false
	}

	return env, true
}

// switchGroup switches the AP to the planned WLAN group via the per-AP
// wlan-group PATCH — but only when the AP is not already on that group, because
// the controller rejects a switch to the current group ("cannot be the current
// wlan group"). This keeps Create idempotent for an AP already bound to the
// desired group (e.g. adopting the production baseline) and tolerates
// out-of-band drift that already matches the plan.
//
// It returns (ok, didSwitch). When the AP is already on the desired group it
// copies the just-read live state into *plan and returns (true, false) so the
// caller can skip a redundant overview read.
func (r *apWlanGroupResource) switchGroup(ctx context.Context, diags *diag.Diagnostics, plan *apWlanGroupResourceModel, action string) (ok bool, didSwitch bool) {
	current := *plan
	if !readOverview(ctx, diags, r, &current) {
		return false, false
	}
	if current.WlanGroupId.IsNull() {
		// The AP is gone upstream; a nonexistent AP cannot be switched.
		diags.AddError("Error "+action, fmt.Sprintf("AP %s was not found on the controller.", plan.ApMac.ValueString()))
		return false, false
	}
	if current.WlanGroupId.ValueString() == plan.WlanGroupId.ValueString() {
		// Already on the desired group; the switch endpoint would reject a no-op.
		// Adopt the fresh live read so the caller needs no second overview read.
		*plan = current
		return true, false
	}

	_, httpResp, callErr := r.client.ApAPI.ModifyApWlanGroup(ctx, r.omadacId, plan.SiteId.ValueString(), plan.ApMac.ValueString()).
		ApUpdateWlanGroupOpenApiVO(expandSwitch(*plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, action)
	if !ok {
		return false, false
	}
	if env.hasError() {
		respondAPIError(diags, action, env.ErrorCode, env.Msg)
		return false, false
	}
	return true, true
}

// Create switches the (always-existing) AP to the desired WLAN group and sets
// the initial Terraform state.
func (r *apWlanGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apWlanGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ok, didSwitch := r.switchGroup(ctx, &resp.Diagnostics, &plan, "creating AP wlan group binding")
	if !ok {
		return
	}

	// switchGroup already adopted the live read when no switch was needed; only
	// re-read after an actual switch.
	if didSwitch {
		if !readOverview(ctx, &resp.Diagnostics, r, &plan) {
			return
		}
		if plan.WlanGroupId.IsNull() {
			resp.Diagnostics.AddError(
				"Error creating AP wlan group binding",
				fmt.Sprintf("AP %s was not present after the switch.", plan.ApMac.ValueString()),
			)
			return
		}
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state from the AP overview. When the AP is gone
// upstream (forgotten/removed), readOverview clears wlan_group_id and the
// resource is dropped from state so Terraform plans its removal.
func (r *apWlanGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apWlanGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readOverview(ctx, &resp.Diagnostics, r, &state) {
		return
	}
	if state.WlanGroupId.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update switches the AP to the newly planned WLAN group and sets the updated
// Terraform state. site_id and ap_mac are immutable (RequiresReplace), so the
// only in-place change is wlan_group_id.
func (r *apWlanGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan apWlanGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state apWlanGroupResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteId = state.SiteId
	plan.ApMac = state.ApMac

	ok, didSwitch := r.switchGroup(ctx, &resp.Diagnostics, &plan, "updating AP wlan group binding")
	if !ok {
		return
	}

	if didSwitch {
		if !readOverview(ctx, &resp.Diagnostics, r, &plan) {
			return
		}
		if plan.WlanGroupId.IsNull() {
			resp.Diagnostics.AddError(
				"Error updating AP wlan group binding",
				fmt.Sprintf("AP %s was not present after the switch.", plan.ApMac.ValueString()),
			)
			return
		}
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete is a no-op: an AP is always bound to exactly one WLAN group and cannot
// be unbound via the Open API. The resource is simply dropped from Terraform
// state; the live AP keeps broadcasting its current group. To move an AP back to
// another group, change wlan_group_id rather than destroying the resource.
func (r *apWlanGroupResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState imports an existing AP WLAN group binding. The import ID is
// `<site_id>/<ap_mac>`. The framework follows ImportState with a Read, which
// populates wlan_group_id and ap_name from the controller.
func (r *apWlanGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, "/")
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<ap_mac>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ap_mac"), idParts[1])...)
}

// readOverview fetches the AP overview and refreshes wlan_group_id and ap_name
// on the model in place. site_id and ap_mac are preserved (they key the read).
func readOverview(ctx context.Context, diags *diag.Diagnostics, r *apWlanGroupResource, model *apWlanGroupResourceModel) bool {
	_, httpResp, callErr := r.client.ApAPI.GetOverviewDetail(ctx, r.omadacId, model.SiteId.ValueString(), model.ApMac.ValueString()).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading AP wlan group binding")
	if !ok {
		return false
	}
	// errNotFound (-1001): the AP is gone upstream. Signal it to the caller via a
	// null wlan_group_id sentinel (the attribute is Required, so null never occurs
	// legitimately) so Read can drop the resource from state.
	if env.hasError() && env.ErrorCode != nil && *env.ErrorCode == errNotFound {
		model.WlanGroupId = types.StringNull()
		model.ApName = types.StringNull()
		return true
	}
	if env.hasError() {
		diags.AddError(
			"Error reading AP wlan group binding",
			fmt.Sprintf("Controller rejected the AP overview read for %s, error code %d: %s", model.ApMac.ValueString(), *env.ErrorCode, env.Msg),
		)
		return false
	}

	var ov apOverviewRead
	if err := json.Unmarshal(env.Result, &ov); err != nil {
		diags.AddError("Error reading AP wlan group binding", "Could not decode AP overview: "+err.Error())
		return false
	}

	flattenOverviewRead(model, &ov)
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
