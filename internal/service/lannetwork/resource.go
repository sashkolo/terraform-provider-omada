package lannetwork

import (
	"context"
	"fmt"
	"strings"
	"terraform-provider-omada/internal/client"

	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &lanNetworkResource{}
	_ resource.ResourceWithConfigure   = &lanNetworkResource{}
	_ resource.ResourceWithImportState = &lanNetworkResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &lanNetworkResource{}
}

// lanNetworkResource is the resource implementation.
type lanNetworkResource struct {
	lanNetworkClient
}

// Configure adds the provider configured client to the resource.
func (r *lanNetworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Guard against the provider data not yet being populated.
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
func (r *lanNetworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lan_network"
}

// Schema defines the schema for the resource.
func (r *lanNetworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Omada LAN network backed by a single gateway-served VLAN. " +
			"The Omada gateway terminates the VLAN, owns the gateway IP, and (optionally) serves DHCP/DNS. " +
			"Requires one of: `Site Settings Manager Modify` or `Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"network_id": schema.StringAttribute{
				Description: "LAN network ID assigned by the controller. This is the primary identifier; use it as the import target.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID to create the network in. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "LAN network name. Must contain 1 to 128 characters and be unique within the site.",
				Required:    true,
			},
			"vlan_id": schema.Int32Attribute{
				Description: "802.1Q VLAN tag for this network. Must be in the range 1-4094 and unused by any " +
					"other network or WAN interface. Changing this forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"gateway_subnet": schema.StringAttribute{
				Description: "Gateway address and mask in CIDR (`IP/Mask`) form, e.g. `192.168.199.1/24`. " +
					"Required because the gateway terminates this VLAN.",
				Required: true,
			},
			"domain": schema.StringAttribute{
				Description: "Domain name advertised for this network.",
				Optional:    true,
			},
			"igmp_snoop_enable": schema.BoolAttribute{
				Description: "Enable IGMP snooping on this network. Defaults to `false`.",
				Optional:    true,
				Computed:    true,
			},
			"device_type": schema.Int32Attribute{
				Description: "DHCP server device type. `1` = gateway (the default and the only value this " +
					"resource currently exercises), `0` = external, `2` = switch, `3` = none. " +
					"Changing this forces replacement.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
					int32planmodifier.RequiresReplace(),
				},
			},
			"dhcp_server": schema.SingleNestedAttribute{
				Description: "Gateway-served DHCP configuration. Omit for a VLAN with no DHCP served by the gateway.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"gateway": schema.StringAttribute{
						Description: "DHCP gateway IP handed to clients, e.g. `192.168.199.1`.",
						Optional:    true,
					},
					"ip": schema.StringAttribute{
						Description: "DHCP server IP, typically the gateway IP, e.g. `192.168.199.1`.",
						Required:    true,
					},
					"netmask": schema.StringAttribute{
						Description: "DHCP subnet mask, e.g. `255.255.255.0`.",
						Required:    true,
					},
					"leasetime": schema.Int32Attribute{
						Description: "DHCP lease time in minutes. Must be in the range 2-2880.",
						Required:    true,
					},
					"pri_dns": schema.StringAttribute{
						Description: "Primary DNS server handed to clients.",
						Optional:    true,
					},
					"snd_dns": schema.StringAttribute{
						Description: "Secondary DNS server handed to clients.",
						Optional:    true,
					},
					"ip_range_pool": schema.ListNestedAttribute{
						Description: "DHCP address-pool ranges. Omit for a single-pool /24 served from the subnet.",
						Optional:    true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"start": schema.StringAttribute{
									Description: "First IP in the range, inclusive.",
									Required:    true,
								},
								"end": schema.StringAttribute{
									Description: "Last IP in the range, inclusive.",
									Required:    true,
								},
							},
						},
					},
				},
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *lanNetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lanNetworkResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build the LAN network value from the plan. Port binding is left empty
	// (an empty SelectPortBindingBriefVO) so no switch ports are bound by this
	// resource; the VLAN exists only on the gateway until explicitly wired out.
	param := omada.CreateVlanParamOpenApiVO{
		DeviceConfig: omada.SelectPortBindingBriefVO{},
		LanNetwork:   expandLanNetwork(plan),
	}

	createResp, _, err := r.client.WiredNetworkAPI.ConfirmCreateVlanNetwork(ctx, r.omadacId, plan.SiteId.ValueString()).
		CreateVlanParamOpenApiVO(param).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating LAN network",
			"Could not create LAN network, unexpected error: "+err.Error(),
		)
		return
	}

	if createResp == nil || createResp.ErrorCode == nil || *createResp.ErrorCode != 0 {
		respondAPIError(&resp.Diagnostics, "creating LAN network", createResp.GetMsg())
		return
	}

	if createResp.Result == nil || len(createResp.Result.NetworkIdList) == 0 {
		resp.Diagnostics.AddError(
			"Error creating LAN network",
			"Create succeeded but the controller returned no network ID.",
		)
		return
	}

	plan.NetworkId = types.StringValue(createResp.Result.NetworkIdList[0])

	// Refresh computed fields from the controller so the state reflects exactly
	// what was created (igmp_snoop_enable, device_type, dhcp_server rounding).
	if !readLanNetwork(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *lanNetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lanNetworkResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve site_id and network_id across refresh; readLanNetwork fills the
	// rest. A not-found result drops the resource from state.
	readLanNetwork(ctx, &resp.Diagnostics, r, &state)

	// When the network is gone, readLanNetwork signals by clearing NetworkId;
	// leave resp.State unset so Terraform removes it from state.
	if state.NetworkId.IsNull() {
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *lanNetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan lanNetworkResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// network_id and site_id are immutable in-place; carry them from state.
	var state lanNetworkResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.NetworkId = state.NetworkId
	plan.SiteId = state.SiteId

	skipEnable := true
	param := omada.ModifyVlanParamOpenApiVO{
		DeviceConfig: omada.SelectPortBindingBriefVO{},
		LanNetwork:   expandLanNetwork(plan),
		SkipEnable:   &skipEnable,
	}

	updateResp, _, err := r.client.WiredNetworkAPI.ConfirmModifyVlanNetwork(ctx, r.omadacId, plan.SiteId.ValueString(), plan.NetworkId.ValueString()).
		ModifyVlanParamOpenApiVO(param).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating LAN network",
			"Could not update LAN network, unexpected error: "+err.Error(),
		)
		return
	}

	if updateResp == nil || updateResp.ErrorCode == nil || *updateResp.ErrorCode != 0 {
		respondAPIError(&resp.Diagnostics, "updating LAN network", updateResp.GetMsg())
		return
	}

	if !readLanNetwork(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *lanNetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lanNetworkResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteResp, _, err := r.client.WiredNetworkAPI.DeleteLanNetwork(ctx, r.omadacId, state.SiteId.ValueString(), state.NetworkId.ValueString()).Execute()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting LAN network",
			"Could not delete LAN network, unexpected error: "+err.Error(),
		)
		return
	}

	// -33503 (network does not exist) is a successful delete: the resource is
	// already gone, which is the desired end state.
	if deleteResp == nil || deleteResp.ErrorCode == nil {
		return
	}
	if *deleteResp.ErrorCode != 0 && *deleteResp.ErrorCode != errNetworkNotFound {
		respondAPIError(&resp.Diagnostics, "deleting LAN network", deleteResp.GetMsg())
		return
	}
}

// ImportState imports an existing LAN network. The import ID is
// `<site_id>/<network_id>`. The framework follows ImportState with a Read, which
// fully populates the remaining attributes from the controller.
func (r *lanNetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<network_id>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network_id"), idParts[1])...)
}

// readLanNetwork fetches the LAN network from the controller and refreshes the
// model in place. It returns false on error (already recorded in diags). When
// the network no longer exists, it clears model.NetworkId so the caller can drop
// the resource from state.
func readLanNetwork(ctx context.Context, diags *diag.Diagnostics, r *lanNetworkResource, model *lanNetworkResourceModel) bool {
	getResp, _, err := r.client.WiredNetworkAPI.GetLanNetwork(ctx, r.omadacId, model.SiteId.ValueString(), model.NetworkId.ValueString()).Execute()
	if err != nil {
		diags.AddError(
			"Error reading LAN network",
			"Could not read LAN network ID "+model.NetworkId.ValueString()+": "+err.Error(),
		)
		return false
	}

	if getResp == nil || getResp.ErrorCode == nil {
		diags.AddError(
			"Error reading LAN network",
			"Controller returned an empty response for LAN network ID "+model.NetworkId.ValueString()+".",
		)
		return false
	}

	if *getResp.ErrorCode == errNetworkNotFound {
		// Network is gone upstream: drop from state.
		model.NetworkId = types.StringNull()
		return true
	}

	if *getResp.ErrorCode != 0 {
		diags.AddError(
			"Error reading LAN network",
			fmt.Sprintf("Could not read LAN network ID %s, error code %d: %s", model.NetworkId.ValueString(), *getResp.ErrorCode, getResp.GetMsg()),
		)
		return false
	}

	flattenLanNetwork(model, getResp.Result)
	return true
}

// respondAPIError records a controller-side error (non-zero errorCode) on the
// given diagnostics.
func respondAPIError(diags *diag.Diagnostics, action, msg string) {
	diags.AddError(
		"Error "+action,
		"Controller rejected the request: "+msg,
	)
}
