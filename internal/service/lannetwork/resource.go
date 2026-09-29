package lannetwork

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
					"other network or WAN interface. Changed in place: replacing the network would delete it, " +
					"with everything that references it, before the new one exists (homelab #515).",
				Required: true,
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
				Description: "Enable IGMP snooping on this network. Defaults to `false` on create; when unset, " +
					"an update keeps the live value.",
				Optional: true,
				Computed: true,
				// Without this the value is unknown on update, and the SDK sends it as
				// false, turning snooping off (homelab #515).
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
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
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "creating LAN network")
	if !ok {
		return
	}

	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "creating LAN network", env.ErrorCode, env.Msg)
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
	// network_id is Computed, so it is Unknown (not Null) at create time; the
	// name lookup must run for both, or a create result without an id stores a
	// null network_id (homelab #514).
	if (plan.NetworkId.IsUnknown() || plan.NetworkId.IsNull()) && !awaitFindLanNetworkByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating LAN network",
			"Create did not return an id and the network was not present in the site afterwards.",
		)
		return
	}

	if !awaitReadLanNetwork(ctx, &resp.Diagnostics, r, &plan) {
		// The network exists on the controller: keep it in state (tainted)
		// rather than orphaning it, so a re-apply replaces it instead of failing
		// on a duplicate name or VLAN (homelab #514).
		resp.Diagnostics.Append(tfstate.SaveCreated(ctx, req.Plan, &resp.State, "network_id", plan.NetworkId.ValueString())...)
		resp.Diagnostics.AddError(
			"Error creating LAN network",
			fmt.Sprintf("LAN network %s was created but could not be read back within the retry window. It is kept in "+
				"state as tainted, so the next apply replaces it.", plan.NetworkId.ValueString()),
		)
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

	row, found := findLanNetworkConfirmed(ctx, &resp.Diagnostics, r, &state)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		// Deleted outside Terraform. Setting nothing would keep the prior state,
		// which the framework pre-fills, so the network would silently stay
		// "managed" (homelab #514).
		resp.State.RemoveResource(ctx)
		return
	}
	flattenLanNetworkRead(&state, row)

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

	// Read-modify-write: the settings this resource doesn't model are sent back
	// with their live values, or the update is refused when one can't be
	// carried safely (homelab #515).
	live := findLanNetworkInList(ctx, &resp.Diagnostics, r, &plan)
	if resp.Diagnostics.HasError() {
		return
	}
	if live == nil {
		resp.Diagnostics.AddError("Error updating LAN network",
			fmt.Sprintf("LAN network %s is not in the site's list, so its unmodeled settings can't be preserved.", plan.NetworkId.ValueString()))
		return
	}
	if plan.InterfaceIds == nil && len(live.InterfaceIds) > 0 {
		// Omitting the list would read back the live ports as an inconsistent
		// result, and sending [] would unbind them; refuse before writing.
		resp.Diagnostics.AddError("Error updating LAN network", fmt.Sprintf(
			"interface_ids is unset, but LAN network %q is bound to %d gateway port(s). Set interface_ids to "+
				"the ports it should keep.", live.Name, len(live.InterfaceIds)))
		return
	}
	body := expandLanNetwork(plan)
	if !carryUnmodeled(&body, live, &resp.Diagnostics) {
		return
	}

	_, httpResp, callErr := r.client.WiredNetworkAPI.ModifyLanNetwork(ctx, r.omadacId, plan.SiteId.ValueString(), plan.NetworkId.ValueString()).
		LanNetworkOpenApiVO(body).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "updating LAN network")
	if !ok {
		return
	}

	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "updating LAN network", env.ErrorCode, env.Msg)
		return
	}

	if !awaitReadLanNetwork(ctx, &resp.Diagnostics, r, &plan) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError(
				"Error updating LAN network",
				fmt.Sprintf("LAN network %s was not present in the site after update.", plan.NetworkId.ValueString()),
			)
		}
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
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "deleting LAN network")
	if !ok {
		return
	}

	// -33503 (network does not exist) is a successful delete: the resource is
	// already gone, which is the desired end state. Any other controller error
	// is also success when a confirmed re-list shows the network gone, since
	// not every firmware answers a missing network with that code.
	if env.HasError() && env.Code() != errNetworkNotFound {
		var listDiags diag.Diagnostics
		if _, found := findLanNetworkConfirmed(ctx, &listDiags, r, &state); !listDiags.HasError() && !found {
			return
		}
		envelope.AddAPIError(&resp.Diagnostics, "deleting LAN network", env.ErrorCode, env.Msg)
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
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading LAN network")
	if !ok {
		return nil
	}

	if env.HasError() {
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

// findLanNetworkInList fetches the LAN-network list and returns the entry
// matching the model's network_id (nil if not present). It does not mutate the
// model.
func findLanNetworkInList(ctx context.Context, diags *diag.Diagnostics, r *lanNetworkResource, model *lanNetworkResourceModel) *lanNetworkReadRow {
	data := fetchLanNetworkList(ctx, diags, r, model)
	if diags.HasError() {
		return nil
	}
	want := model.NetworkId.ValueString()
	for i := range data {
		if data[i].Id != nil && *data[i].Id == want {
			return &data[i]
		}
	}
	return nil
}

// findLanNetworkConfirmed looks the model's network_id up in the list. A miss
// is re-checked a few times before it is believed, because the list is
// eventually consistent; a found row is returned at once.
func findLanNetworkConfirmed(ctx context.Context, diags *diag.Diagnostics, r *lanNetworkResource, model *lanNetworkResourceModel) (*lanNetworkReadRow, bool) {
	var row *lanNetworkReadRow
	found, last := retry.Until(ctx, goneConfirmations, retry.Interval, func(d *diag.Diagnostics) bool {
		row = findLanNetworkInList(ctx, d, r, model)
		// Stop on an error too: a failing list is not evidence of absence.
		return row != nil || d.HasError()
	})
	diags.Append(last...)
	return row, found && row != nil
}

// goneConfirmations is how many list reads must miss a network before Read or
// Delete treats it as deleted.
const goneConfirmations = 3

// awaitReadLanNetwork retries the list read until the network is present,
// refreshing the model in place. A freshly created or modified network may take
// a moment to appear in the list. It never clears network_id: the caller
// (Create/Update) already knows the id. Only the last attempt's diagnostics are
// kept, so a transient error doesn't fail the apply.
func awaitReadLanNetwork(ctx context.Context, diags *diag.Diagnostics, r *lanNetworkResource, model *lanNetworkResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		if row := findLanNetworkInList(ctx, d, r, model); row != nil {
			flattenLanNetworkRead(model, row)
			return true
		}
		return false
	})
	diags.Append(last...)
	return ok
}

// awaitFindLanNetworkByName retries the name-based list lookup until the network
// is present, setting model.NetworkId. Used after create when the create result
// omitted the id. Names are unique within a site, so a match is the network
// just created.
func awaitFindLanNetworkByName(ctx context.Context, diags *diag.Diagnostics, r *lanNetworkResource, model *lanNetworkResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		data := fetchLanNetworkList(ctx, d, r, model)
		if d.HasError() {
			return false
		}
		for i := range data {
			if data[i].Name == model.Name.ValueString() && data[i].Id != nil && *data[i].Id != "" {
				model.NetworkId = types.StringValue(*data[i].Id)
				return true
			}
		}
		return false
	})
	diags.Append(last...)
	return ok
}
