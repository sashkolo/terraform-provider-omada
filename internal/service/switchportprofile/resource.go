package switchportprofile

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &switchPortProfileResource{}
	_ resource.ResourceWithConfigure   = &switchPortProfileResource{}
	_ resource.ResourceWithImportState = &switchPortProfileResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &switchPortProfileResource{}
}

// switchPortProfileResource is the resource implementation.
type switchPortProfileResource struct {
	switchPortProfileClient
}

// Configure adds the provider configured client to the resource.
func (r *switchPortProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *switchPortProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch_port_profile"
}

// Schema defines the schema for the resource.
func (r *switchPortProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a reusable Omada switch (OSW) LAN/port profile: the object a switch " +
			"port is assigned to, which defines the port's VLAN membership. A profile with a " +
			"`native_network_id` and no `tagged_network_ids` is an access/untagged port on that " +
			"VLAN; adding tagged networks makes it a trunk. Targets the Open API v1 profile " +
			"surface (`/openapi/v1/.../lan-profiles`) implemented by controller firmware such as " +
			"5.15.x (the v2 surface returns 404 there). Requires one of: `Site Settings Manager " +
			"Modify` or `Network Config Page Modify`.",
		Attributes: map[string]schema.Attribute{
			"profile_id": schema.StringAttribute{
				Description: "LAN-profile ID assigned by the controller. Use it (with site_id) as the import target.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				Description: "Site ID the profile belongs to. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Profile name. Must contain 1 to 128 characters and be unique within the site.",
				Required:    true,
			},
			"native_network_id": schema.StringAttribute{
				Description: "Native (untagged/PVID) LAN network ID for ports using this profile. Must not " +
					"also appear in tagged_network_ids. Omit for a profile with no native VLAN.",
				Optional: true,
			},
			"tagged_network_ids": schema.ListAttribute{
				Description: "Tagged (trunk) LAN network IDs carried by this profile. Set to `[]` (the " +
					"default) for an access port that carries no tagged VLANs.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"untagged_network_ids": schema.ListAttribute{
				Description: "Additional untagged LAN network IDs carried by this profile. Set to `[]` " +
					"(the default) for a simple access port.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"poe": schema.Int32Attribute{
				Description: "PoE mode: `0` on, `1` off, `2` do-not-modify (the default).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"dot1x": schema.Int32Attribute{
				Description: "802.1X mode: `0` force-unauthorized, `1` force-authorized, `2` auto (the default).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"bandwidth_ctrl_type": schema.Int32Attribute{
				Description: "Bandwidth control type: `0` off (the default), `1` rate limit, `2` storm control.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"port_isolation_enable": schema.BoolAttribute{
				Description: "Whether port isolation is enabled. Defaults to `false`.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"lldp_med_enable": schema.BoolAttribute{
				Description: "Whether LLDP-MED is enabled.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"loopback_detect_enable": schema.BoolAttribute{
				Description: "Whether port-based loopback detection is enabled.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"spanning_tree_enable": schema.BoolAttribute{
				Description: "Whether spanning tree is enabled on ports using this profile.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"flag": schema.Int32Attribute{
				Description: "Read-only profile origin flag: `0` default (All/Disable/LAN), `1` native " +
					"(generated by a LAN network), `2` user-created.",
				Computed: true,
			},
			"type": schema.Int32Attribute{
				Description: "Read-only profile type: `0` LAN profile-ALL, `1` LAN profile-Disable, " +
					"`2` any other LAN profile.",
				Computed: true,
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

// Create creates the resource and sets the initial Terraform state.
func (r *switchPortProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan switchPortProfileResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WiredNetworkAPI.CreateLanProfile(ctx, r.omadacId, plan.SiteId.ValueString()).
		LanProfileConfigOpenApiVO(expandProfile(ctx, plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "creating switch port profile")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "creating switch port profile", env.ErrorCode, env.Msg)
		return
	}

	// Prefer the id from the create result when present; otherwise locate the
	// newly-created profile by name (names are unique within a site).
	if len(env.Result) > 0 {
		var cr createResult
		if err := json.Unmarshal(env.Result, &cr); err == nil && cr.Id != nil && *cr.Id != "" {
			plan.ProfileId = types.StringValue(*cr.Id)
		}
	}
	if plan.ProfileId.IsNull() && !findProfileByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating switch port profile",
			"Create did not return an id and the profile was not present in the site afterwards.",
		)
		return
	}

	if !readProfile(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state. The controller exposes no single-profile
// GET, so Read fetches the paged profile list and selects the entry matching
// profile_id.
func (r *switchPortProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state switchPortProfileResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readProfile(ctx, &resp.Diagnostics, r, &state) {
		return
	}

	// When the profile is gone upstream, readProfile clears ProfileId; leave
	// resp.State unset so Terraform drops it from state.
	if state.ProfileId.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *switchPortProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan switchPortProfileResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state switchPortProfileResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ProfileId = state.ProfileId
	plan.SiteId = state.SiteId

	_, httpResp, callErr := r.client.WiredNetworkAPI.ModifyLanProfile(ctx, r.omadacId, plan.SiteId.ValueString(), plan.ProfileId.ValueString()).
		LanProfileConfigOpenApiVO(expandProfile(ctx, plan)).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "updating switch port profile")
	if !ok {
		return
	}
	if env.hasError() {
		respondAPIError(&resp.Diagnostics, "updating switch port profile", env.ErrorCode, env.Msg)
		return
	}

	if !readProfile(ctx, &resp.Diagnostics, r, &plan) {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *switchPortProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state switchPortProfileResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WiredNetworkAPI.DeleteLanProfile(ctx, r.omadacId, state.SiteId.ValueString(), state.ProfileId.ValueString()).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, &resp.Diagnostics, "deleting switch port profile")
	if !ok {
		return
	}

	// A "profile does not exist" code is a successful delete: the resource is
	// already gone, which is the desired end state.
	if env.hasError() && env.ErrorCode != nil && *env.ErrorCode != errProfileNotFound {
		respondAPIError(&resp.Diagnostics, "deleting switch port profile", env.ErrorCode, env.Msg)
		return
	}
}

// ImportState imports an existing switch port profile. The import ID is
// `<site_id>/<profile_id>`. The framework follows ImportState with a Read, which
// fully populates the remaining attributes from the controller.
func (r *switchPortProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.SplitN(req.ID, "/", 2)
	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>/<profile_id>`, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("profile_id"), idParts[1])...)
}

// fetchProfileList fetches the paged LAN-profile list for the model's site and
// decodes it leniently.
func fetchProfileList(ctx context.Context, diags *diag.Diagnostics, r *switchPortProfileResource, model *switchPortProfileResourceModel) []profileReadRow {
	_, httpResp, callErr := r.client.WiredNetworkAPI.GetLanProfileList(ctx, r.omadacId, model.SiteId.ValueString()).
		Page(1).PageSize(1000).Execute()
	env, ok := decodeEnvelope(httpResp, callErr, diags, "reading switch port profile")
	if !ok {
		return nil
	}

	if env.hasError() {
		diags.AddError(
			"Error reading switch port profile",
			fmt.Sprintf("Controller rejected the list for site %s, error code %d: %s", model.SiteId.ValueString(), *env.ErrorCode, env.Msg),
		)
		return nil
	}

	var lr listResult
	if err := json.Unmarshal(env.Result, &lr); err != nil {
		diags.AddError("Error reading switch port profile", "Could not decode LAN-profile list: "+err.Error())
		return nil
	}

	return lr.Data
}

// findProfileByName locates the profile matching the model's name (used after
// create when the id is not returned). Sets model.ProfileId on success; returns
// false if not found.
func findProfileByName(ctx context.Context, diags *diag.Diagnostics, r *switchPortProfileResource, model *switchPortProfileResourceModel) bool {
	data := fetchProfileList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range data {
		if data[i].Name == model.Name.ValueString() {
			model.ProfileId = types.StringPointerValue(data[i].Id)
			return true
		}
	}

	return false
}

// readProfile selects the list entry matching the model's profile_id and
// refreshes the model in place. When the profile no longer exists, it clears
// model.ProfileId so the caller can drop the resource from state.
func readProfile(ctx context.Context, diags *diag.Diagnostics, r *switchPortProfileResource, model *switchPortProfileResourceModel) bool {
	data := fetchProfileList(ctx, diags, r, model)
	if diags.HasError() {
		return false
	}

	for i := range data {
		row := &data[i]
		if row.Id != nil && *row.Id == model.ProfileId.ValueString() {
			flattenProfileRead(model, row)
			return true
		}
	}

	// Not present in the list: the profile is gone upstream.
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
