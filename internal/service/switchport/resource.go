package switchport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"terraform-provider-omada/internal/client"

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
	_ resource.Resource                = &switchPortResource{}
	_ resource.ResourceWithConfigure   = &switchPortResource{}
	_ resource.ResourceWithImportState = &switchPortResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &switchPortResource{}
}

// switchPortResource is the resource implementation.
type switchPortResource struct {
	switchPortClient
}

// Configure adds the provider configured client to the resource.
func (r *switchPortResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *switchPortResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch_port"
}

// Schema defines the schema for the resource.
func (r *switchPortResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the per-port assignment on a managed Omada switch: which reusable port " +
			"profile the port uses (and therefore its VLAN posture), plus the port name, PoE mode, " +
			"and admin state. A physical port always exists, so this behaves like a singleton keyed " +
			"by (site_id, switch_mac, port): Create/Update apply the settings via the per-port " +
			"Modify endpoint and Delete is a no-op. Targets the Open API v1 switch surface " +
			"(`GET /switches/{mac}` read, `PATCH /switches/{mac}/ports/{port}` write) implemented by " +
			"controller firmware such as 5.15.x. The port's VLAN membership is governed by the " +
			"referenced `omada_switch_port_profile`. Requires `Site Device Manager Modify`.",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				Description: "Site ID the switch belongs to. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"switch_mac": schema.StringAttribute{
				Description: "MAC address of the switch (e.g. `E4-FA-C4-9E-CD-87`). Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"port": schema.Int32Attribute{
				Description: "Physical port number on the switch (for a non-stacked switch, port `8` is " +
					"the UI's `1/0/8`). Changing this forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"profile_id": schema.StringAttribute{
				Description: "ID of the omada_switch_port_profile the port is assigned to. This selects " +
					"the port's VLAN posture (access vs trunk, native/tagged VLANs).",
				Required: true,
			},
			"name": schema.StringAttribute{
				Description: "Port name/description.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"profile_override_enable": schema.BoolAttribute{
				Description: "Whether the port uses a custom fill mode (per-port override) on top of the " +
					"profile. `false` means the port strictly follows the profile. Defaults to the " +
					"controller value on import.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"poe": schema.Int32Attribute{
				Description: "PoE mode: `0` off, `1` on (802.3at/af), `2` do-not-modify.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"disabled": schema.BoolAttribute{
				Description: "Whether the port is administratively disabled. Defaults to `false` (enabled).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"profile_name": schema.StringAttribute{
				Description: "Read-only name of the assigned profile, as reported by the controller.",
				Computed:    true,
			},
			"lag_port": schema.BoolAttribute{
				Description: "Read-only: whether this port participates in a link-aggregation group.",
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

// modifyPort applies the plan to the live port via the per-port Modify endpoint.
func (r *switchPortResource) modifyPort(ctx context.Context, diags *diag.Diagnostics, plan switchPortResourceModel, action string) bool {
	port := strconv.FormatInt(int64(plan.Port.ValueInt32()), 10)
	_, httpResp, callErr := r.client.SwitchAPI.ModifySwitchPort(ctx, r.omadacId, plan.SiteId.ValueString(), plan.SwitchMac.ValueString(), port).
		OswPortSettingVO(expandPort(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, action)
	if !ok {
		return false
	}
	if env.hasError() {
		respondAPIError(diags, action, env.ErrorCode, env.Msg)
		return false
	}
	return true
}

// Create applies the desired settings to the (always-existing) port and sets
// the initial Terraform state.
func (r *switchPortResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan switchPortResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.modifyPort(ctx, &resp.Diagnostics, plan, "creating switch port") {
		return
	}

	if !readPort(ctx, &resp.Diagnostics, r, &plan) {
		return
	}
	if plan.ProfileId.IsNull() {
		resp.Diagnostics.AddError(
			"Error creating switch port",
			fmt.Sprintf("Port %d was not present on switch %s after modify.", plan.Port.ValueInt32(), plan.SwitchMac.ValueString()),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state from the switch overview portList.
func (r *switchPortResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state switchPortResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readPort(ctx, &resp.Diagnostics, r, &state) {
		return
	}

	// When the port is gone upstream (switch forgotten), readPort clears
	// ProfileId; drop the resource from state.
	if state.ProfileId.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update applies the changed settings and sets the updated Terraform state.
func (r *switchPortResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan switchPortResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state switchPortResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteId = state.SiteId
	plan.SwitchMac = state.SwitchMac
	plan.Port = state.Port

	if !r.modifyPort(ctx, &resp.Diagnostics, plan, "updating switch port") {
		return
	}

	if !readPort(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete is a no-op: a physical switch port cannot be removed. The resource is
// simply dropped from Terraform state; the live port keeps its last-applied
// configuration.
func (r *switchPortResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState imports an existing switch port. The import ID is
// `<site_id>/<switch_mac>/<port>`. The framework follows ImportState with a
// Read, which fully populates the remaining attributes from the controller.
func (r *switchPortResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, "/")
	if len(idParts) != 3 || idParts[0] == "" || idParts[1] == "" || idParts[2] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<switch_mac>/<port>`, got %q.", req.ID),
		)
		return
	}

	portNum, err := strconv.ParseInt(idParts[2], 10, 32)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Port segment %q of import ID must be an integer: %s", idParts[2], err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("switch_mac"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("port"), int32(portNum))...)
}

// readPort fetches the switch overview and selects the portList entry matching
// the model's port, refreshing the model in place. When the port is absent (the
// switch is gone), it clears model.ProfileId so the caller can drop the resource.
func readPort(ctx context.Context, diags *diag.Diagnostics, r *switchPortResource, model *switchPortResourceModel) bool {
	_, httpResp, callErr := r.client.SwitchAPI.GetSwitchInfo(ctx, r.omadacId, model.SiteId.ValueString(), model.SwitchMac.ValueString()).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading switch port")
	if !ok {
		return false
	}
	if env.hasError() {
		diags.AddError(
			"Error reading switch port",
			fmt.Sprintf("Controller rejected the switch read for %s, error code %d: %s", model.SwitchMac.ValueString(), *env.ErrorCode, env.Msg),
		)
		return false
	}

	var sr switchOverviewResult
	if err := json.Unmarshal(env.Result, &sr); err != nil {
		diags.AddError("Error reading switch port", "Could not decode switch overview: "+err.Error())
		return false
	}

	for i := range sr.PortList {
		row := &sr.PortList[i]
		if row.Port == model.Port.ValueInt32() {
			flattenPortRead(model, row)
			return true
		}
	}

	// Port not present: the switch is gone upstream.
	model.ProfileId = types.StringNull()
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
