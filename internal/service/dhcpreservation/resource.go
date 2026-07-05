package dhcpreservation

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// gridPageSize is the page size used when reading the reservation grid. The
// endpoint requires page + pageSize; the resource pages through until it has
// seen totalRows so it never misses a reservation on a busy site.
const gridPageSize int32 = 100

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &reservationResource{}
	_ resource.ResourceWithConfigure   = &reservationResource{}
	_ resource.ResourceWithImportState = &reservationResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &reservationResource{}
}

// reservationResource is the resource implementation.
type reservationResource struct {
	reservationClient
}

// Configure adds the provider configured client to the resource.
func (r *reservationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *reservationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_reservation"
}

// Schema defines the schema for the resource.
func (r *reservationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Omada gateway DHCP fixed-IP reservation: a MAC-keyed binding that hands a " +
			"specific client a stable address from a managed LAN network. Targets the Open API v1 " +
			"reservation surface (`/openapi/v1/.../setting/service/dhcp`) implemented by controller " +
			"firmware such as 5.15.x and 6.2.x. The reservation is addressed by MAC on modify/delete, so " +
			"the import id is `<site_id>/<mac>`. Requires one of: `Site Settings Manager Modify` or " +
			"`Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"reservation_id": schema.StringAttribute{
				Description: "Reservation ID assigned by the controller. Informational only: the reservation " +
					"is addressed by MAC, not by this id.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID the reservation belongs to. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mac": schema.StringAttribute{
				Description: "Device MAC address, format `AA-BB-CC-11-22-33`. This is the reservation key; " +
					"changing it forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip": schema.StringAttribute{
				Description: "Reserved IP address. Must lie in the target network's subnet; keep it outside " +
					"the dynamic DHCP pool to avoid lease conflicts.",
				Required: true,
			},
			"net_id": schema.StringAttribute{
				Description: "ID of the managed LAN network the reservation belongs to (the `network_id` " +
					"attribute of the corresponding `omada_lan_network`).",
				Required: true,
			},
			"net_name": schema.StringAttribute{
				Description: "Name of the LAN network the reservation belongs to, as reported by the " +
					"controller.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Description: "Optional reservation description (1 to 128 characters). Do not store secrets here.",
				Optional:    true,
			},
			"status": schema.BoolAttribute{
				Description: "Whether the reservation is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
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
	// net/http guarantees a non-nil Body for a non-nil Response, but a mocked
	// transport or SDK anomaly could return one with a nil Body; guard the read.
	if httpResp.Body == nil {
		diags.AddError("Error "+action, "Controller returned a response with no body.")
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
			msg += fmt.Sprintf(" (original error: %s)", callErr.Error())
		}
		diags.AddError("Error "+action, msg)
		return omadaEnvelope{}, false
	}

	// A transport/HTTP error (non-2xx) whose body decoded but carries no
	// errorCode (e.g. a reverse-proxy error page shaped as JSON) would otherwise
	// slip past hasError() and be treated as success; surface it instead.
	if callErr != nil && env.ErrorCode == nil {
		diags.AddError("Error "+action, "API call failed: "+callErr.Error())
		return omadaEnvelope{}, false
	}

	return env, true
}

// Create creates the resource and sets the initial Terraform state.
func (r *reservationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan reservationResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ServiceAPI.CreateDhcpReservation(ctx, r.omadacId, plan.SiteId.ValueString()).
		CreateDhcpReservationOpenApiVO(expandReservation(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating DHCP reservation")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating DHCP reservation", env.ErrorCode, env.Msg)
		return
	}

	// Adopt the id from the create result when present; the authoritative state
	// is refreshed from the grid read below, which is keyed by MAC.
	if len(env.Result) > 0 {
		var cr createResult
		if err := json.Unmarshal(env.Result, &cr); err == nil && cr.Id != nil && *cr.Id != "" {
			plan.ReservationId = types.StringValue(*cr.Id)
		}
	}

	if !readReservation(ctx, &resp.Diagnostics, r, &plan) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError(
				"Error creating DHCP reservation",
				fmt.Sprintf("The reservation for MAC %s was not present on the site after create.", plan.Mac.ValueString()),
			)
		}
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state. The controller exposes no single-
// reservation GET, so Read pages the reservation grid and selects the entry
// matching the MAC.
func (r *reservationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state reservationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readReservation(ctx, &resp.Diagnostics, r, &state) {
		if resp.Diagnostics.HasError() {
			return
		}
		// Gone upstream: drop it from state so Terraform plans a re-create.
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *reservationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan reservationResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state reservationResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// site_id and mac are the immutable key (RequiresReplace); carry them and the
	// controller-assigned id forward from prior state.
	plan.SiteId = state.SiteId
	plan.Mac = state.Mac
	plan.ReservationId = state.ReservationId

	_, httpResp, callErr := r.client.ServiceAPI.ModifyDhcpReservation(ctx, r.omadacId, plan.SiteId.ValueString(), plan.Mac.ValueString()).
		CreateDhcpReservationOpenApiVO(expandReservation(plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating DHCP reservation")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating DHCP reservation", env.ErrorCode, env.Msg)
		return
	}

	if !readReservation(ctx, &resp.Diagnostics, r, &plan) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError(
				"Error updating DHCP reservation",
				fmt.Sprintf("The reservation for MAC %s was not present on the site after update.", plan.Mac.ValueString()),
			)
		}
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *reservationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state reservationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.ServiceAPI.DeleteDhcpReservation(ctx, r.omadacId, state.SiteId.ValueString(), state.Mac.ValueString()).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "deleting DHCP reservation")
	if !ok {
		return
	}

	if env.hasError() {
		// A delete may fail because the reservation is already gone (e.g. removed
		// out-of-band). The error-code table is not documented for this endpoint,
		// so confirm absence by re-reading the grid rather than guessing a code:
		// if the MAC is no longer present, the desired end state (gone) holds.
		probe := state
		var probeDiags diag.Diagnostics
		found := readReservation(ctx, &probeDiags, r, &probe)
		if !probeDiags.HasError() && !found {
			return
		}
		// The probe itself failed (or the reservation is still present); surface
		// the original delete error, plus any probe error context so a network or
		// auth failure during the re-read is not silently dropped.
		resp.Diagnostics.Append(probeDiags...)
		respondAPIError(&resp.Diagnostics, "deleting DHCP reservation", env.ErrorCode, env.Msg)
		return
	}
}

// ImportState imports an existing reservation. The import ID is
// `<site_id>/<mac>`. The framework follows ImportState with a Read, which
// populates the remaining attributes from the controller.
func (r *reservationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<mac>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), idParts[1])...)
}

// fetchReservationList pages the reservation grid for the model's site and
// decodes each page leniently, accumulating the rows. The endpoint requires
// page + pageSize; paging continues until the accumulated count reaches
// totalRows (or a short page is returned).
func fetchReservationList(ctx context.Context, diags *diag.Diagnostics, r *reservationResource, model *reservationResourceModel) []reservationReadRow {
	var rows []reservationReadRow
	for page := int32(1); ; page++ {
		_, httpResp, callErr := r.client.ServiceAPI.GetDhcpReservationGrid(ctx, r.omadacId, model.SiteId.ValueString()).
			Page(page).PageSize(gridPageSize).Execute()
		env, ok := decodeEnvelope(httpResp, callErr, diags, "reading DHCP reservation")
		if !ok {
			return nil
		}
		if env.hasError() {
			diags.AddError(
				"Error reading DHCP reservation",
				fmt.Sprintf("Controller rejected the reservation grid for site %s, error code %d: %s", model.SiteId.ValueString(), *env.ErrorCode, env.Msg),
			)
			return nil
		}
		if len(env.Result) == 0 {
			return rows
		}

		var grid reservationGridResult
		if err := json.Unmarshal(env.Result, &grid); err != nil {
			diags.AddError("Error reading DHCP reservation", "Could not decode reservation grid: "+err.Error())
			return nil
		}

		rows = append(rows, grid.Data...)

		// Stop when this page was not full (last page) or we have collected the
		// full set. A missing totalRows falls back to the short-page signal.
		if len(grid.Data) < int(gridPageSize) {
			return rows
		}
		if grid.TotalRows != nil && int64(len(rows)) >= *grid.TotalRows {
			return rows
		}
	}
}

// readReservation selects the grid entry matching the model's MAC and refreshes
// the model in place. Returns true when the reservation was found. A false
// return with no diagnostics means the reservation is gone upstream; a false
// return with diagnostics means the read failed.
func readReservation(ctx context.Context, diags *diag.Diagnostics, r *reservationResource, model *reservationResourceModel) bool {
	rows := fetchReservationList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	want := normalizeMAC(model.Mac.ValueString())
	for i := range rows {
		row := &rows[i]
		if row.Mac != nil && normalizeMAC(*row.Mac) == want {
			flattenReservationRead(model, row)
			return true
		}
	}

	return false
}

// normalizeMAC lowercases a MAC and strips the common separators so matching is
// insensitive to both case and delimiter style (`AA-BB-...`, `aa:bb:...`,
// `aabb...`). The controller returns dash-separated uppercase MACs and the
// schema documents that format, but normalizing keeps Read/import from silently
// missing a reservation the operator entered with a different separator.
func normalizeMAC(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", ":", "", ".", "").Replace(s))
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
