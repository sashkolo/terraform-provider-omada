package lannetwork

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
		Description: "Manages an Omada LAN network backed by a single gateway-served VLAN. " +
			"The Omada gateway terminates the VLAN (purpose \"interface\"), owns the gateway IP, " +
			"binds to gateway LAN ports, and (optionally) serves DHCP/DNS. Targets the Open API v1 " +
			"LAN-network surface implemented by controller firmware such as 5.15.x. Requires one of: " +
			"`Site Settings Manager Modify` or `Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"network_id": schema.StringAttribute{
				Description: "LAN network ID assigned by the controller. Use it (with site_id) as the import target.",
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
			"purpose": schema.Int32Attribute{
				Description: "LAN network purpose. `1` = interface (the default; a gateway-terminated network with " +
					"a gateway_subnet), `0` = VLAN only. Changing this forces replacement.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
					int32planmodifier.RequiresReplace(),
				},
			},
			"gateway_subnet": schema.StringAttribute{
				Description: "Gateway address and mask in CIDR (`IP/Mask`) form, e.g. `192.168.199.1/24`. " +
					"Required for purpose `interface`; the gateway terminates this VLAN.",
				Required: true,
			},
			"interface_ids": schema.ListAttribute{
				Description: "Gateway LAN port IDs the network binds to (from the controller's WAN/LAN status " +
					"endpoint). Required for purpose `interface`; the controller rejects creation with no ports.",
				ElementType: types.StringType,
				Optional:    true,
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
			"dhcp_settings": schema.SingleNestedAttribute{
				Description: "Gateway-served DHCP configuration. Omit for a VLAN with no DHCP served by the gateway.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"enable": schema.BoolAttribute{
						Description: "Whether the DHCP server is enabled.",
						Optional:    true,
					},
					"dhcpns": schema.StringAttribute{
						Description: "DHCP server selection: `auto` or `manual`.",
						Optional:    true,
					},
					"gateway": schema.StringAttribute{
						Description: "DHCP gateway IP handed to clients, e.g. `192.168.199.1`.",
						Optional:    true,
					},
					"ipaddr_start": schema.StringAttribute{
						Description: "First IP in the DHCP range, inclusive.",
						Optional:    true,
					},
					"ipaddr_end": schema.StringAttribute{
						Description: "Last IP in the DHCP range, inclusive.",
						Optional:    true,
					},
					"leasetime": schema.Int32Attribute{
						Description: "DHCP lease time in minutes. Must be in the range 2-2880.",
						Optional:    true,
					},
					"pri_dns": schema.StringAttribute{
						Description: "Primary DNS server handed to clients.",
						Optional:    true,
					},
					"snd_dns": schema.StringAttribute{
						Description: "Secondary DNS server handed to clients.",
						Optional:    true,
					},
				},
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
func (r *lanNetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lanNetworkResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WiredNetworkAPI.CreateLanNetwork(ctx, r.omadacId, plan.SiteId.ValueString()).
		LanNetworkOpenApiVO(expandLanNetwork(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating LAN network")
	if !ok {
		return
	}

	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating LAN network", env.ErrorCode, env.Msg)
		return
	}

	// Prefer the id from the create result when present; otherwise locate the
	// newly-created network by name (names are unique within a site).
	if len(env.Result) > 0 {
		var cr createResult
		if err := json.Unmarshal(env.Result, &cr); err == nil && cr.Id != nil && *cr.Id != "" {
			plan.NetworkId = types.StringValue(*cr.Id)
		}
	}
	if plan.NetworkId.IsNull() && !findLanNetworkByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating LAN network",
			"Create did not return an id and the network was not present in the site afterwards.",
		)
		return
	}

	if !readLanNetwork(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data. The controller exposes
// no single-network GET, so Read fetches the paged LAN-network list and selects
// the entry matching network_id.
func (r *lanNetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lanNetworkResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readLanNetwork(ctx, &resp.Diagnostics, r, &state)

	// When the network is gone upstream, readLanNetwork clears NetworkId; leave
	// resp.State unset so Terraform drops it from state.
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

	var state lanNetworkResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.NetworkId = state.NetworkId
	plan.SiteId = state.SiteId

	_, httpResp, callErr := r.client.WiredNetworkAPI.ModifyLanNetwork(ctx, r.omadacId, plan.SiteId.ValueString(), plan.NetworkId.ValueString()).
		LanNetworkOpenApiVO(expandLanNetwork(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating LAN network")
	if !ok {
		return
	}

	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating LAN network", env.ErrorCode, env.Msg)
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

	_, httpResp, callErr := r.client.WiredNetworkAPI.DeleteLanNetwork(ctx, r.omadacId, state.SiteId.ValueString(), state.NetworkId.ValueString()).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "deleting LAN network")
	if !ok {
		return
	}

	// -33503 (network does not exist) is a successful delete: the resource is
	// already gone, which is the desired end state.
	if env.hasError() && env.ErrorCode != nil && *env.ErrorCode != errNetworkNotFound {
		respondAPIError(&resp.Diagnostics, "deleting LAN network", env.ErrorCode, env.Msg)
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

// fetchLanNetworkList fetches the paged LAN-network list for the model's site and
// decodes it leniently.
func fetchLanNetworkList(ctx context.Context, diags *diag.Diagnostics, r *lanNetworkResource, model *lanNetworkResourceModel) []lanNetworkReadRow {
	_, httpResp, callErr := r.client.WiredNetworkAPI.GetLanNetworkList(ctx, r.omadacId, model.SiteId.ValueString()).
		Page(1).PageSize(1000).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading LAN network")
	if !ok {
		return nil
	}

	if env.hasError() {
		diags.AddError(
			"Error reading LAN network",
			fmt.Sprintf("Controller rejected the list for site %s, error code %d: %s", model.SiteId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return nil
	}

	var lr listResult
	if err := json.Unmarshal(env.Result, &lr); err != nil {
		diags.AddError("Error reading LAN network", "Could not decode LAN-network list: "+err.Error())
		return nil
	}

	return lr.Data
}

// findLanNetworkByName locates the network matching the model's name (used after
// create when the id is not returned). Sets model.NetworkId on success; returns
// false if not found.
func findLanNetworkByName(ctx context.Context, diags *diag.Diagnostics, r *lanNetworkResource, model *lanNetworkResourceModel) bool {
	data := fetchLanNetworkList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range data {
		if data[i].Name == model.Name.ValueString() {
			model.NetworkId = types.StringPointerValue(data[i].Id)
			return true
		}
	}

	return false
}

// readLanNetwork selects the list entry matching the model's network_id and
// refreshes the model in place. When the network no longer exists, it clears
// model.NetworkId so the caller can drop the resource from state.
func readLanNetwork(ctx context.Context, diags *diag.Diagnostics, r *lanNetworkResource, model *lanNetworkResourceModel) bool {
	data := fetchLanNetworkList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range data {
		row := &data[i]
		if row.Id != nil && *row.Id == model.NetworkId.ValueString() {
			flattenLanNetworkRead(model, row)
			return true
		}
	}

	// Not present in the list: the network is gone upstream.
	model.NetworkId = types.StringNull()
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
