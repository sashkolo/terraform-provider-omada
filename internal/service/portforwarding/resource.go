package portforwarding

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"terraform-provider-omada/internal/client"
	"terraform-provider-omada/internal/envelope"
	"terraform-provider-omada/internal/retry"
	"terraform-provider-omada/internal/tfstate"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const listPageSize int32 = 1000

// maxListPages bounds the paging loop: a controller that ignores `page` and
// answers every request with a full page and no totalRows would otherwise be
// paged forever (homelab #514). 100 pages is 100,000 rules, far past any site.
const maxListPages int32 = 100

var (
	_ resource.Resource                   = &portForwardingResource{}
	_ resource.ResourceWithConfigure      = &portForwardingResource{}
	_ resource.ResourceWithImportState    = &portForwardingResource{}
	_ resource.ResourceWithValidateConfig = &portForwardingResource{}
)

func NewResource() resource.Resource {
	return &portForwardingResource{}
}

type portForwardingResource struct {
	portForwardingClient
}

func (r *portForwardingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *portForwardingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_forwarding"
}

func (r *portForwardingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Omada gateway port-forwarding rule. The resource deliberately excludes DMZ " +
			"because DMZ exposes every port; use explicit port/range mappings instead. Targets the Open API v1 " +
			"NAT surface (`/openapi/v1/.../nat/port-forwardings`) on controller firmware such as 6.2.x. " +
			"Import with `<site_id>/<port_forwarding_id>`. Requires one of: `Site Settings Manager Modify` or " +
			"`Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"port_forwarding_id": schema.StringAttribute{
				Description: "Port-forwarding rule ID assigned by the controller.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID the rule belongs to. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Rule name, 1 to 64 characters and unique within the site.",
				Required:    true,
			},
			"status": schema.BoolAttribute{
				Description: "Whether the rule is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"external_port": schema.StringAttribute{
				Description: "WAN/source port or inclusive range, e.g. `51820` or `8000-8010`.",
				Required:    true,
			},
			"forward_ip": schema.StringAttribute{
				Description: "Destination IPv4 address on the LAN.",
				Required:    true,
			},
			"forward_port": schema.StringAttribute{
				Description: "Destination port or inclusive range, e.g. `51820` or `9000-9010`.",
				Required:    true,
			},
			"protocol": schema.Int32Attribute{
				Description: "Transport protocol: `0` = TCP+UDP, `1` = TCP, `2` = UDP.",
				Required:    true,
			},
			"source_addresses": schema.ListAttribute{
				Description: "Optional external source-address allowlist. An empty list allows any source; a " +
					"non-empty list makes Omada use its limited-address mode.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"wan_port_ids": schema.ListAttribute{
				Description: "Physical WAN port IDs to accept the mapping on. IDs come from Omada's internet " +
					"basic-info surface. Leave empty only when intentionally using another WAN selector.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"virtual_wan_ids": schema.ListAttribute{
				Description: "Optional virtual-WAN IDs on which to accept the mapping.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"wan_ips": schema.MapAttribute{
				Description: "Optional WAN-address selector as a map of WAN port ID to WAN IP.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ValidateConfig catches the dangerous/ambiguous mistakes locally before the
// controller sees a request. The controller remains authoritative for its WAN
// selector identifiers and source-address syntax.
func (r *portForwardingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config portForwardingResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		length := len([]rune(config.Name.ValueString()))
		if length < 1 || length > 64 {
			resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid port-forwarding name", "Name must contain 1 to 64 characters.")
		}
	}
	validatePortAttribute(&resp.Diagnostics, path.Root("external_port"), config.ExternalPort)
	validatePortAttribute(&resp.Diagnostics, path.Root("forward_port"), config.ForwardPort)

	if !config.ForwardIp.IsNull() && !config.ForwardIp.IsUnknown() {
		ip := net.ParseIP(config.ForwardIp.ValueString())
		if ip == nil || ip.To4() == nil {
			resp.Diagnostics.AddAttributeError(path.Root("forward_ip"), "Invalid forwarding address", "forward_ip must be an IPv4 address.")
		}
	}
	if !config.Protocol.IsNull() && !config.Protocol.IsUnknown() {
		protocol := config.Protocol.ValueInt32()
		if protocol < 0 || protocol > 2 {
			resp.Diagnostics.AddAttributeError(path.Root("protocol"), "Invalid forwarding protocol", "protocol must be 0 (TCP+UDP), 1 (TCP), or 2 (UDP).")
		}
	}

	if !config.WanIps.IsNull() && !config.WanIps.IsUnknown() {
		for wanID, value := range config.WanIps.Elements() {
			ip, ok := value.(types.String)
			if !ok || ip.IsNull() || ip.IsUnknown() {
				continue
			}
			parsed := net.ParseIP(ip.ValueString())
			if parsed == nil || parsed.To4() == nil {
				resp.Diagnostics.AddAttributeError(path.Root("wan_ips").AtMapKey(wanID), "Invalid WAN address", "WAN IP selectors must be IPv4 addresses.")
			}
		}
	}
}

func validatePortAttribute(diags *diag.Diagnostics, attributePath path.Path, value types.String) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	parts := strings.Split(value.ValueString(), "-")
	if len(parts) < 1 || len(parts) > 2 {
		diags.AddAttributeError(attributePath, "Invalid port specification", "Use a port from 1 to 65535 or an inclusive range such as 8000-8010.")
		return
	}
	ports := make([]int, 0, len(parts))
	for _, part := range parts {
		port, err := strconv.Atoi(part)
		if err != nil || port < 1 || port > 65535 {
			diags.AddAttributeError(attributePath, "Invalid port specification", "Use a port from 1 to 65535 or an inclusive range such as 8000-8010.")
			return
		}
		ports = append(ports, port)
	}
	if len(ports) == 2 && ports[0] > ports[1] {
		diags.AddAttributeError(attributePath, "Invalid port range", "The first port in a range must not exceed the last port.")
	}
}

func (r *portForwardingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portForwardingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.NATAPI.CreatePortForwarding(ctx, r.omadacId, plan.SiteId.ValueString()).
		PortForwardingConfig(expandPortForwarding(plan)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "creating port forwarding")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "creating port forwarding", env.ErrorCode, env.Msg)
		return
	}

	if len(env.Result) > 0 {
		var result createResult
		if err := json.Unmarshal(env.Result, &result); err == nil {
			switch {
			case result.PortForwardingId != nil && *result.PortForwardingId != "":
				plan.PortForwardingId = types.StringValue(*result.PortForwardingId)
			case result.Id != nil && *result.Id != "":
				plan.PortForwardingId = types.StringValue(*result.Id)
			}
		}
	}

	if (plan.PortForwardingId.IsUnknown() || plan.PortForwardingId.IsNull()) && !awaitFindByName(ctx, &resp.Diagnostics, r, &plan) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError("Error creating port forwarding", "Create succeeded but the rule id could not be recovered from the site list.")
		}
		return
	}
	if !awaitRead(ctx, &resp.Diagnostics, r, &plan) {
		// The rule exists on the controller and its id is known: keep it in
		// state (tainted) rather than orphaning an open port forward, so the next
		// apply replaces it instead of failing on a duplicate name (homelab #514).
		resp.Diagnostics.Append(tfstate.SaveCreated(ctx, req.Plan, &resp.State, "port_forwarding_id", plan.PortForwardingId.ValueString())...)
		resp.Diagnostics.AddError(
			"Error creating port forwarding",
			fmt.Sprintf("Port forwarding %s was created but could not be read back within the retry window. It is kept "+
				"in state as tainted, so the next apply replaces it.", plan.PortForwardingId.ValueString()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *portForwardingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portForwardingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	row, found := findByIDConfirmed(ctx, &resp.Diagnostics, r, &state)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	if row.DMZ {
		resp.Diagnostics.AddError("Unsupported DMZ rule", "The imported Omada rule enables DMZ (all ports). omada_port_forwarding intentionally manages explicit port/range mappings only.")
		return
	}
	flattenPortForwarding(&state, row)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *portForwardingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portForwardingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state portForwardingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteId = state.SiteId
	plan.PortForwardingId = state.PortForwardingId

	_, httpResp, callErr := r.client.NATAPI.ModifyPortForwarding(ctx, r.omadacId, plan.SiteId.ValueString(), plan.PortForwardingId.ValueString()).
		PortForwardingConfig(expandPortForwarding(plan)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "updating port forwarding")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "updating port forwarding", env.ErrorCode, env.Msg)
		return
	}
	if !awaitRead(ctx, &resp.Diagnostics, r, &plan) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError("Error updating port forwarding", "The rule could not be read back after update.")
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *portForwardingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portForwardingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.NATAPI.DeletePortForwarding(ctx, r.omadacId, state.SiteId.ValueString(), state.PortForwardingId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "deleting port forwarding")
	if !ok {
		return
	}
	// A rejected delete of a rule that is already gone is the desired end
	// state. The not-found code isn't documented for this endpoint, so confirm
	// against the list instead of trusting a code (homelab #514).
	if env.HasError() {
		var listDiags diag.Diagnostics
		if _, found := findByIDConfirmed(ctx, &listDiags, r, &state); !listDiags.HasError() && !found {
			return
		}
		envelope.AddAPIError(&resp.Diagnostics, "deleting port forwarding", env.ErrorCode, env.Msg)
	}
}

func (r *portForwardingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Unexpected Import ID", fmt.Sprintf("Expected import ID in the form `<site_id>/<port_forwarding_id>`, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("port_forwarding_id"), parts[1])...)
}

func fetchList(ctx context.Context, diags *diag.Diagnostics, r *portForwardingResource, model *portForwardingResourceModel) []portForwardingReadRow {
	rows := make([]portForwardingReadRow, 0)
	for page := int32(1); page <= maxListPages; page++ {
		_, httpResp, callErr := r.client.NATAPI.GetPortForwardingList(ctx, r.omadacId, model.SiteId.ValueString()).
			Page(page).PageSize(listPageSize).Execute()
		env, ok := envelope.Decode(httpResp, callErr, diags, "reading port forwarding")
		if !ok {
			return nil
		}
		if env.HasError() {
			envelope.AddAPIError(diags, "reading port forwarding", env.ErrorCode, env.Msg)
			return nil
		}
		var result listResult
		if err := json.Unmarshal(env.Result, &result); err != nil {
			diags.AddError("Error reading port forwarding", "Could not decode port-forwarding list: "+err.Error())
			return nil
		}
		rows = append(rows, result.Data...)
		if listComplete(len(result.Data), len(rows), result.TotalRows) {
			return rows
		}
	}
	diags.AddError("Error reading port forwarding",
		fmt.Sprintf("The port-forwarding list did not end within %d pages of %d rows; the controller may be ignoring "+
			"the page parameter.", maxListPages, listPageSize))
	return nil
}

// listComplete determines whether a paged list has been exhausted. Some
// controller versions omit totalRows, so a full page must be treated as
// evidence that another page may exist; a short page is the fallback end
// marker. When totalRows is present, reaching it also ends pagination.
func listComplete(pageRows, accumulatedRows int, totalRows *int64) bool {
	return pageRows < int(listPageSize) || (totalRows != nil && int64(accumulatedRows) >= *totalRows)
}

func findByID(ctx context.Context, diags *diag.Diagnostics, r *portForwardingResource, model *portForwardingResourceModel) *portForwardingReadRow {
	rows := fetchList(ctx, diags, r, model)
	if diags.HasError() {
		return nil
	}
	for index := range rows {
		if rows[index].Id != nil && *rows[index].Id == model.PortForwardingId.ValueString() {
			return &rows[index]
		}
	}
	return nil
}

// findByIDConfirmed looks the model's id up in the list. A miss is re-checked
// a few times before it is believed, because the list is eventually
// consistent; a found row is returned at once.
func findByIDConfirmed(ctx context.Context, diags *diag.Diagnostics, r *portForwardingResource, model *portForwardingResourceModel) (*portForwardingReadRow, bool) {
	var row *portForwardingReadRow
	found, last := retry.Until(ctx, goneConfirmations, retry.Interval, func(d *diag.Diagnostics) bool {
		row = findByID(ctx, d, r, model)
		// Stop on an error too: a failing list is not evidence of absence.
		return row != nil || d.HasError()
	})
	diags.Append(last...)
	return row, found && row != nil
}

// goneConfirmations is how many list reads must miss a rule before Read or
// Delete treats it as deleted.
const goneConfirmations = 3

// awaitFindByName retries the name-based list lookup until the rule is present,
// setting model.PortForwardingId. Only the last attempt's diagnostics are kept,
// except that an ambiguous name stops the retries at once.
func awaitFindByName(ctx context.Context, diags *diag.Diagnostics, r *portForwardingResource, model *portForwardingResourceModel) bool {
	found := false
	_, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		rows := fetchList(ctx, d, r, model)
		if d.HasError() {
			return false
		}
		var match *portForwardingReadRow
		for index := range rows {
			if rows[index].Name != model.Name.ValueString() {
				continue
			}
			if match != nil {
				d.AddError("Ambiguous port-forwarding name", fmt.Sprintf("More than one rule named %q exists in site %s; the create id cannot be recovered safely.", model.Name.ValueString(), model.SiteId.ValueString()))
				return true // permanent: retrying cannot resolve it
			}
			match = &rows[index]
		}
		if match == nil || match.Id == nil || *match.Id == "" {
			return false
		}
		model.PortForwardingId = types.StringValue(*match.Id)
		found = true
		return true
	})
	diags.Append(last...)
	return found
}

// awaitRead retries the list read until the rule is present and reflects the
// model, refreshing the model in place. It never clears the id. Only the last
// attempt's diagnostics are kept, except that a DMZ row stops the retries.
func awaitRead(ctx context.Context, diags *diag.Diagnostics, r *portForwardingResource, model *portForwardingResourceModel) bool {
	found := false
	_, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		row := findByID(ctx, d, r, model)
		if row == nil {
			return false
		}
		if row.DMZ {
			d.AddError("Unsupported DMZ rule", "The controller returned the managed rule with DMZ enabled; refusing to represent an all-ports exposure as omada_port_forwarding.")
			return true // permanent: retrying cannot resolve it
		}
		if !rowMatchesModel(row, model) {
			return false
		}
		flattenPortForwarding(model, row)
		found = true
		return true
	})
	diags.Append(last...)
	return found
}
