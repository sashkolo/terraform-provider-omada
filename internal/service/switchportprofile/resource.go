package switchportprofile

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
			"5.15.x (the v2 surface returns 404 there). Deleting a profile that a switch port " +
			"still uses is refused: move the ports first, in an earlier apply or with " +
			"`create_before_destroy` on the profile, because Terraform deletes the profile before " +
			"it updates the ports. Requires one of: `Site Settings Manager Modify` or `Network " +
			"Config Page Modify`, plus `Site Device Manager View Only` to check the ports.",
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

// Create creates the resource and sets the initial Terraform state.
func (r *switchPortProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan switchPortProfileResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The SDK sends these toggles as plain booleans, so an unset one would
	// create the profile with that protection off rather than at the
	// controller's default. Require them on create; an imported
	// profile keeps its live values through UseStateForUnknown.
	for name, v := range map[string]types.Bool{
		"spanning_tree_enable":   plan.SpanningTreeEnable,
		"loopback_detect_enable": plan.LoopbackDetectEnable,
		"port_isolation_enable":  plan.PortIsolationEnable,
		"lldp_med_enable":        plan.LldpMedEnable,
	} {
		if v.IsNull() || v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Missing switch port profile setting",
				name+" must be set when creating a profile: left unset, the profile would be created with it "+
					"off, not at the controller's default.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := r.client.WiredNetworkAPI.CreateLanProfile(ctx, r.omadacId, plan.SiteId.ValueString()).
		LanProfileConfigOpenApiVO(expandProfile(ctx, plan)).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "creating switch port profile")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "creating switch port profile", env.ErrorCode, env.Msg)
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
	// profile_id is Computed, so it is Unknown (not Null) at create time. Treat
	// both as "not yet known" so the name-based fallback runs.
	if (plan.ProfileId.IsUnknown() || plan.ProfileId.IsNull()) && !awaitFindProfileByName(ctx, &resp.Diagnostics, r, &plan) {
		resp.Diagnostics.AddError(
			"Error creating switch port profile",
			"Create did not return an id and the profile was not present in the site afterwards.",
		)
		return
	}

	if !awaitReadProfile(ctx, &resp.Diagnostics, r, &plan) {
		// The profile exists on the controller: keep it in state (tainted)
		// with the id the POST returned rather than a null id.
		resp.Diagnostics.Append(tfstate.SaveCreated(ctx, req.Plan, &resp.State, "profile_id", plan.ProfileId.ValueString())...)
		resp.Diagnostics.AddError(
			"Error creating switch port profile",
			fmt.Sprintf("Switch port profile %s was created but could not be read back within the retry window. It is "+
				"kept in state as tainted, so the next apply replaces it.", plan.ProfileId.ValueString()),
		)
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

	row, found := findProfileConfirmed(ctx, &resp.Diagnostics, r, &state)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		// Gone upstream (confirmed by repeated misses, as the list is
		// eventually consistent): drop it so Terraform plans a re-create.
		resp.State.RemoveResource(ctx)
		return
	}
	flattenProfileRead(&state, row)

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
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "updating switch port profile")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "updating switch port profile", env.ErrorCode, env.Msg)
		return
	}

	if !awaitReadProfile(ctx, &resp.Diagnostics, r, &plan) {
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

	if r.refuseDeleteInUse(ctx, &resp.Diagnostics, &state) {
		return
	}

	_, httpResp, callErr := r.client.WiredNetworkAPI.DeleteLanProfile(ctx, r.omadacId, state.SiteId.ValueString(), state.ProfileId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "deleting switch port profile")
	if !ok {
		return
	}

	// A "profile does not exist" code is a successful delete: the resource is
	// already gone, which is the desired end state.
	if env.HasError() && env.Code() != errProfileNotFound {
		// Any other controller error is still success when a confirmed re-list
		// shows the profile absent.
		var listDiags diag.Diagnostics
		if _, found := findProfileConfirmed(ctx, &listDiags, r, &state); !listDiags.HasError() && !found {
			return
		}
		envelope.AddAPIError(&resp.Diagnostics, "deleting switch port profile", env.ErrorCode, env.Msg)
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
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading switch port profile")
	if !ok {
		return nil
	}

	if env.HasError() {
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

// findProfileInList fetches the list and returns the entry matching the
// model's profile_id (nil if not present). It does not mutate the model.
func findProfileInList(ctx context.Context, diags *diag.Diagnostics, r *switchPortProfileResource, model *switchPortProfileResourceModel) *profileReadRow {
	data := fetchProfileList(ctx, diags, r, model)
	if diags.HasError() {
		return nil
	}
	want := model.ProfileId.ValueString()
	for i := range data {
		if data[i].Id != nil && *data[i].Id == want {
			return &data[i]
		}
	}
	return nil
}

// findProfileConfirmed looks the model's profile_id up in the list. A miss is
// re-checked a few times before it is believed, because the list is eventually
// consistent; a found row is returned at once.
func findProfileConfirmed(ctx context.Context, diags *diag.Diagnostics, r *switchPortProfileResource, model *switchPortProfileResourceModel) (*profileReadRow, bool) {
	var row *profileReadRow
	found, last := retry.Until(ctx, goneConfirmations, retry.Interval, func(d *diag.Diagnostics) bool {
		row = findProfileInList(ctx, d, r, model)
		// Stop on an error too: a failing list is not evidence of absence.
		return row != nil || d.HasError()
	})
	diags.Append(last...)
	return row, found && row != nil
}

// goneConfirmations is how many list reads must miss a profile before Read or
// Delete treats it as deleted.
const goneConfirmations = 3

// awaitReadProfile retries the list read until the profile is present,
// refreshing the model in place. It never clears profile_id: Create and Update
// already know the id. Only the last attempt's diagnostics are
// kept.
func awaitReadProfile(ctx context.Context, diags *diag.Diagnostics, r *switchPortProfileResource, model *switchPortProfileResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		if row := findProfileInList(ctx, d, r, model); row != nil {
			flattenProfileRead(model, row)
			return true
		}
		return false
	})
	diags.Append(last...)
	return ok
}

// awaitFindProfileByName retries the name-based list lookup until the profile
// is present, setting model.ProfileId. Used after create when the create
// response omitted the id; names are unique within a site.
func awaitFindProfileByName(ctx context.Context, diags *diag.Diagnostics, r *switchPortProfileResource, model *switchPortProfileResourceModel) bool {
	ok, last := retry.Until(ctx, retry.Attempts, retry.Interval, func(d *diag.Diagnostics) bool {
		data := fetchProfileList(ctx, d, r, model)
		if d.HasError() {
			return false
		}
		for i := range data {
			if data[i].Id != nil && *data[i].Id != "" && data[i].Name == model.Name.ValueString() {
				model.ProfileId = types.StringValue(*data[i].Id)
				return true
			}
		}
		return false
	})
	diags.Append(last...)
	return ok
}
