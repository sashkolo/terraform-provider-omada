package ipmacbinding

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// gridPageSize is the page size used when reading the binding grid. The
// endpoint requires page + pageSize; Read pages until it has seen totalRows.
const gridPageSize int32 = 100

// goneConfirmations is how many grid reads must miss a binding before Read
// treats it as deleted.
const goneConfirmations = 3

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &bindingResource{}
	_ resource.ResourceWithConfigure   = &bindingResource{}
	_ resource.ResourceWithImportState = &bindingResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &bindingResource{}
}

type bindingResource struct {
	bindingClient
}

// Configure adds the provider configured client to the resource.
func (r *bindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *bindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gateway_ip_mac_binding"
}

// Schema defines the schema for the resource.
func (r *bindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one entry of the gateway's IP-MAC binding table (Device Config > Gateway > " +
			"IP-MAC Binding in the controller UI). The gateway keeps a static ARP entry for each bound " +
			"pair, so it can still reach a client whose dynamic ARP entry has expired, for example a " +
			"sleeping host woken by a directed wake-on-LAN packet. Targets the Open API v1 surface " +
			"(`/openapi/v1/.../sites/{siteId}/ip-mac-binds`). The controller addresses an entry by its " +
			"binding id; the import id is `<site_id>/<mac>` and Read resolves the id. This resource " +
			"never reads or changes the site-wide ARP spoofing defense setting, so it can't restrict " +
			"the gateway to bound clients only. Requires one of: `Site Settings Manager Modify` or " +
			"`Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"binding_id": schema.StringAttribute{
				Description: "Binding ID assigned by the controller.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID the binding belongs to. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mac": schema.StringAttribute{
				Description: "Client MAC address, format `AA-BB-CC-11-22-33`. Case and separators " +
					"(`-`, `:`, `.`, none) are ignored when matching the controller's entry. Changing it " +
					"updates the entry in place.",
				Required:   true,
				Validators: []validator.String{macValidator{}},
			},
			"ip": schema.StringAttribute{
				Description: "IP address bound to the MAC. It must lie in the subnet of the interface.",
				Required:    true,
			},
			"interface_type": schema.StringAttribute{
				Description: "Type of the gateway interface the binding applies to: `lan` (a LAN network) " +
					"or `wan` (a WAN port). Defaults to `lan`.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString(interfaceTypeLAN),
				Validators: []validator.String{oneOfValidator{values: []string{interfaceTypeLAN, interfaceTypeWAN}}},
			},
			"interface_id": schema.StringAttribute{
				Description: "ID of the interface: the `network_id` of an `omada_lan_network` when " +
					"`interface_type` is `lan`, or a WAN port ID when it is `wan`.",
				Required: true,
			},
			"description": schema.StringAttribute{
				Description: "Optional description, 1 to 64 characters. Do not store secrets here.",
				Optional:    true,
				Validators:  []validator.String{lengthValidator{min: 1, max: 64}},
			},
			"status": schema.BoolAttribute{
				Description: "Whether the binding is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"export_to_dhcp_reservation": schema.BoolAttribute{
				Description: "Whether the controller also exports the pair to the DHCP reservation table " +
					"(\"Auto-export to DHCP reservation\" in the UI). Defaults to false. The exported " +
					"reservation is not managed by this resource; manage reservations with " +
					"`omada_dhcp_reservation` instead.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}

// Create creates the binding. It first refuses a MAC or IP that another entry
// already binds, naming that entry, so an existing binding is imported rather
// than duplicated or overwritten.
func (r *bindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan bindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rows := fetchBindingList(ctx, &resp.Diagnostics, r, plan.SiteId.ValueString())
	if resp.Diagnostics.HasError() {
		return
	}
	if conflict := findConflict(rows, plan.Mac.ValueString(), plan.Ip.ValueString(), ""); conflict != "" {
		resp.Diagnostics.AddError("IP-MAC binding conflict", conflict+
			fmt.Sprintf(" To manage the existing entry, import it with the id `%s/<mac>`.", plan.SiteId.ValueString()))
		return
	}

	_, httpResp, callErr := r.client.IPMACBindingAPI.CreateIpMacBinding(ctx, r.omadacId, plan.SiteId.ValueString()).
		IPMacBinding(expandBinding(plan, false)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "creating IP-MAC binding")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "creating IP-MAC binding", env.ErrorCode, env.Msg)
		return
	}

	// The create endpoint returns no id: resolve it from the grid by MAC.
	plan.BindingId = types.StringNull()
	if !awaitReadBinding(ctx, &resp.Diagnostics, r, &plan) {
		// The POST succeeded, so the entry exists: keep it in state (tainted)
		// under its MAC, which Delete resolves to the id, rather than orphaning
		// it. The next apply then replaces it instead of failing on a conflict.
		resp.Diagnostics.Append(tfstate.SaveCreated(ctx, req.Plan, &resp.State, "mac", plan.Mac.ValueString())...)
		resp.Diagnostics.AddError(
			"Error creating IP-MAC binding",
			fmt.Sprintf("The binding for MAC %s was created but could not be read back within the retry window. "+
				"It is kept in state as tainted, so the next apply replaces it.", plan.Mac.ValueString()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the state from the grid. The controller has no single-entry
// GET, so Read pages the grid and selects the entry by binding id, or by MAC
// when the id isn't known yet (after import).
func (r *bindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state bindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A miss is re-checked before it is believed, because the grid is
	// eventually consistent; a list error is not evidence of absence.
	found, last := retry.Until(ctx, goneConfirmations, retry.Interval, func(d *diag.Diagnostics) bool {
		return readBinding(ctx, d, r, &state) || d.HasError()
	})
	resp.Diagnostics.Append(last...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update modifies the entry in place, addressed by its binding id.
func (r *bindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state bindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteId = state.SiteId
	plan.BindingId = state.BindingId

	id := r.resolveID(ctx, &resp.Diagnostics, state)
	if resp.Diagnostics.HasError() {
		return
	}
	if id == "" {
		resp.Diagnostics.AddError("Error updating IP-MAC binding",
			fmt.Sprintf("The binding for MAC %s is no longer on the site.", state.Mac.ValueString()))
		return
	}
	plan.BindingId = types.StringValue(id)

	rows := fetchBindingList(ctx, &resp.Diagnostics, r, plan.SiteId.ValueString())
	if resp.Diagnostics.HasError() {
		return
	}
	if conflict := findConflict(rows, plan.Mac.ValueString(), plan.Ip.ValueString(), id); conflict != "" {
		resp.Diagnostics.AddError("IP-MAC binding conflict", conflict)
		return
	}

	clearDescription := plan.Description.IsNull() && !state.Description.IsNull()
	_, httpResp, callErr := r.client.IPMACBindingAPI.ModifyIpMacBinding(ctx, r.omadacId, plan.SiteId.ValueString(), id).
		IPMacBinding(expandBinding(plan, clearDescription)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "updating IP-MAC binding")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "updating IP-MAC binding", env.ErrorCode, env.Msg)
		return
	}

	if !awaitReadBinding(ctx, &resp.Diagnostics, r, &plan) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError("Error updating IP-MAC binding",
				fmt.Sprintf("The binding %s was not present on the site after update.", id))
		}
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete removes the entry. A tainted entry saved without an id is resolved by
// MAC first; an entry that is already gone counts as deleted.
func (r *bindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state bindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := r.resolveID(ctx, &resp.Diagnostics, state)
	if resp.Diagnostics.HasError() || id == "" {
		return
	}

	_, httpResp, callErr := r.client.IPMACBindingAPI.DeleteIpMacBinding(ctx, r.omadacId, state.SiteId.ValueString(), id).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "deleting IP-MAC binding")
	if !ok {
		return
	}
	if env.HasError() {
		// The error codes aren't documented for this endpoint, so confirm
		// absence by re-reading the grid rather than guessing a code.
		probe := state
		probe.BindingId = types.StringValue(id)
		var probeDiags diag.Diagnostics
		found := readBinding(ctx, &probeDiags, r, &probe)
		if !probeDiags.HasError() && !found {
			return
		}
		resp.Diagnostics.Append(probeDiags...)
		envelope.AddAPIError(&resp.Diagnostics, "deleting IP-MAC binding", env.ErrorCode, env.Msg)
	}
}

// ImportState imports an existing binding by `<site_id>/<mac>`. The framework
// follows with a Read, which resolves the binding id and every other field.
func (r *bindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || !validMAC(parts[1]) {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<mac>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), parts[1])...)
}

// resolveID returns the binding id from state, or looks it up by MAC when state
// has none (a tainted create). "" with no error means the entry is gone.
func (r *bindingResource) resolveID(ctx context.Context, diags *diag.Diagnostics, state bindingResourceModel) string {
	if id := state.BindingId.ValueString(); !state.BindingId.IsNull() && !state.BindingId.IsUnknown() && id != "" {
		return id
	}
	probe := state
	if !readBinding(ctx, diags, r, &probe) {
		return ""
	}
	return probe.BindingId.ValueString()
}

// fetchBindingList pages the binding grid for a site and decodes each page
// leniently.
func fetchBindingList(ctx context.Context, diags *diag.Diagnostics, r *bindingResource, siteID string) []bindingReadRow {
	var rows []bindingReadRow
	for page := int32(1); ; page++ {
		_, httpResp, callErr := r.client.IPMACBindingAPI.GetGridIpMacBinding(ctx, r.omadacId, siteID).
			Page(page).PageSize(gridPageSize).Execute()
		env, ok := envelope.Decode(httpResp, callErr, diags, "reading IP-MAC bindings")
		if !ok {
			return nil
		}
		if env.HasError() {
			envelope.AddAPIError(diags, "reading IP-MAC bindings", env.ErrorCode, env.Msg)
			return nil
		}
		if len(env.Result) == 0 {
			return rows
		}

		var grid bindingGridResult
		if err := json.Unmarshal(env.Result, &grid); err != nil {
			diags.AddError("Error reading IP-MAC bindings", "Could not decode the binding grid: "+err.Error())
			return nil
		}
		rows = append(rows, grid.Data...)

		if len(grid.Data) < int(gridPageSize) {
			return rows
		}
		if grid.TotalRows != nil && int64(len(rows)) >= *grid.TotalRows {
			return rows
		}
	}
}

// readBinding selects the model's entry from the grid, by binding id when the
// model has one and by MAC otherwise, and refreshes the model in place. A false
// return with no diagnostics means the entry is gone.
func readBinding(ctx context.Context, diags *diag.Diagnostics, r *bindingResource, model *bindingResourceModel) bool {
	rows := fetchBindingList(ctx, diags, r, model.SiteId.ValueString())
	if diags.HasError() {
		return false
	}
	if row := selectRow(rows, *model); row != nil {
		flattenBindingRead(model, row)
		return true
	}
	return false
}

func selectRow(rows []bindingReadRow, model bindingResourceModel) *bindingReadRow {
	if id := model.BindingId.ValueString(); !model.BindingId.IsNull() && !model.BindingId.IsUnknown() && id != "" {
		for i := range rows {
			if rows[i].Id != nil && *rows[i].Id == id {
				return &rows[i]
			}
		}
		return nil
	}
	want := normalizeMAC(model.Mac.ValueString())
	for i := range rows {
		if rows[i].Mac != nil && normalizeMAC(*rows[i].Mac) == want {
			return &rows[i]
		}
	}
	return nil
}

// awaitReadBinding retries readBinding until the entry is present. Only the
// last attempt's diagnostics are kept.
func awaitReadBinding(ctx context.Context, diags *diag.Diagnostics, r *bindingResource, model *bindingResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		return readBinding(ctx, d, r, model)
	})
	diags.Append(last...)
	return ok
}

// findConflict describes the first entry, other than selfID, that already binds
// the MAC or the IP, or returns "".
func findConflict(rows []bindingReadRow, mac, ip, selfID string) string {
	wantMAC := normalizeMAC(mac)
	for _, row := range rows {
		if row.Id != nil && selfID != "" && *row.Id == selfID {
			continue
		}
		id := ""
		if row.Id != nil {
			id = *row.Id
		}
		if row.Mac != nil && normalizeMAC(*row.Mac) == wantMAC {
			return fmt.Sprintf("MAC %s is already bound by entry %s.", mac, id)
		}
		if row.Ip != nil && *row.Ip == ip {
			return fmt.Sprintf("IP %s is already bound by entry %s.", ip, id)
		}
	}
	return ""
}

// normalizeMAC lowercases a MAC and strips the common separators, so matching
// ignores case and delimiter style (`AA-BB-...`, `aa:bb:...`, `aabb...`).
func normalizeMAC(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", ":", "", ".", "").Replace(s))
}

var macHex = regexp.MustCompile(`^[0-9a-f]{12}$`)

func validMAC(s string) bool {
	return macHex.MatchString(normalizeMAC(s))
}

// macValidator accepts a MAC of 12 hex digits in any of the separator styles
// normalizeMAC understands.
type macValidator struct{}

func (macValidator) Description(context.Context) string {
	return "value must be a MAC address such as AA-BB-CC-11-22-33"
}
func (v macValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (v macValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validMAC(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid MAC address",
			fmt.Sprintf("%q is not a MAC address; use the form AA-BB-CC-11-22-33.", req.ConfigValue.ValueString()))
	}
}

type oneOfValidator struct{ values []string }

func (v oneOfValidator) Description(context.Context) string {
	return "value must be one of: " + strings.Join(v.values, ", ")
}
func (v oneOfValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (v oneOfValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, ok := range v.values {
		if req.ConfigValue.ValueString() == ok {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", v.Description(ctx)+".")
}

type lengthValidator struct{ min, max int }

func (v lengthValidator) Description(context.Context) string {
	return fmt.Sprintf("length must be between %d and %d characters", v.min, v.max)
}
func (v lengthValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (v lengthValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if n := len([]rune(req.ConfigValue.ValueString())); n < v.min || n > v.max {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid length", v.Description(ctx)+".")
	}
}
