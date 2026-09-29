package switchport

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"terraform-provider-omada/internal/client"
	"terraform-provider-omada/internal/envelope"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// errDeviceNotFound is GetSwitchInfo's "This device does not exist" code: the
// switch was removed from the controller, so its ports are gone too.
const errDeviceNotFound int32 = -39050

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
			"allow_trunk_reassign": schema.BoolAttribute{
				Description: "Allow moving the port off a trunk profile (the \"All\" type, or any profile with " +
					"tagged networks). Without it such a write is refused, because uplinks and AP trunks sit " +
					"on those profiles. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"allow_lag_member": schema.BoolAttribute{
				Description: "Allow writing a port that is a link-aggregation member; the write takes it out of " +
					"its LAG. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}

// modifyPort applies the plan to the live port via the per-port Modify endpoint.
func (r *switchPortResource) modifyPort(ctx context.Context, diags *diag.Diagnostics, plan switchPortResourceModel, action string) bool {
	if !r.guardPortWrite(ctx, diags, plan, action) {
		return false
	}
	port := strconv.FormatInt(int64(plan.Port.ValueInt32()), 10)
	_, httpResp, callErr := r.client.SwitchAPI.ModifySwitchPort(ctx, r.omadacId, plan.SiteId.ValueString(), plan.SwitchMac.ValueString(), port).
		OswPortSettingVO(expandPort(plan)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, action)
	if !ok {
		return false
	}
	if env.HasError() {
		envelope.AddAPIError(diags, action, env.ErrorCode, env.Msg)
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

	// State written by 0.15.0 or earlier has no guard switches; default them
	// here so upgrading plans a no-op instead of a write to the port.
	if state.AllowTrunkReassign.IsNull() {
		state.AllowTrunkReassign = types.BoolValue(false)
	}
	if state.AllowLagMember.IsNull() {
		state.AllowLagMember = types.BoolValue(false)
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
	if plan.ProfileId.IsNull() {
		resp.Diagnostics.AddError(
			"Error updating switch port",
			fmt.Sprintf("Port %d was not present on switch %s after modify.", plan.Port.ValueInt32(), plan.SwitchMac.ValueString()),
		)
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
	// The guard switches are config-only; an import starts from their defaults
	// so it plans to a no-op.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("allow_trunk_reassign"), false)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("allow_lag_member"), false)...)
}

// readPort fetches the switch overview and selects the portList entry matching
// the model's port, refreshing the model in place. When the port is absent, or
// the switch itself is gone from the controller, it clears model.ProfileId so
// the caller can drop the resource.
func readPort(ctx context.Context, diags *diag.Diagnostics, r *switchPortResource, model *switchPortResourceModel) bool {
	_, httpResp, callErr := r.client.SwitchAPI.GetSwitchInfo(ctx, r.omadacId, model.SiteId.ValueString(), model.SwitchMac.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading switch port")
	if !ok {
		return false
	}
	if env.Code() == errDeviceNotFound {
		// The switch was removed from the controller. Erroring here would fail
		// every refresh forever; the port cannot exist without its switch, so
		// report it gone (homelab #514).
		model.ProfileId = types.StringNull()
		return true
	}
	if env.HasError() {
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
