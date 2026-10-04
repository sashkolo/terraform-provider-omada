package upnpsetting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"terraform-provider-omada/internal/client"
	"terraform-provider-omada/internal/envelope"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &upnpSettingResource{}
	_ resource.ResourceWithConfigure   = &upnpSettingResource{}
	_ resource.ResourceWithImportState = &upnpSettingResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &upnpSettingResource{}
}

// upnpSettingResource is the resource implementation.
type upnpSettingResource struct {
	upnpClient
}

// Configure adds the provider configured client to the resource.
func (r *upnpSettingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*client.Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Meta, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = data.Client
	r.omadacId = data.OmadacId
}

// Metadata returns the resource type name.
func (r *upnpSettingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_upnp_setting"
}

// Schema defines the schema for the resource.
func (r *upnpSettingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the site's gateway UPnP setting: whether UPnP is on, and on which LAN " +
			"networks and WAN ports. This is a singleton per site: the object always exists, so import " +
			"it (`<site_id>`) to adopt the live setting before managing it. Create and Update send the " +
			"whole setting; Delete only removes it from Terraform state and leaves the controller's " +
			"setting as it is (destroying the resource does not turn UPnP off; set `enable = false` " +
			"and apply for that).",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				Description: "Site ID the setting belongs to; the import target. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enable": schema.BoolAttribute{
				Description: "Whether UPnP is enabled on the gateway.",
				Required:    true,
			},
			"network_ids": schema.SetAttribute{
				Description: "IDs of the LAN networks UPnP serves. When unset, the live selection is kept; " +
					"set `[]` to declare that none is selected (the controller leaves an empty selection out " +
					"of its answer, and it reads as `[]`).",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
			},
			"wan_port_ids": schema.SetAttribute{
				Description: "IDs of the WAN ports UPnP maps on. When unset, the live selection is kept; " +
					"set `[]` to declare that none is selected (the controller leaves an empty selection out " +
					"of its answer, and it reads as `[]`).",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
			},
			"support_by_ds_lite_and_map_e": schema.BoolAttribute{
				Description: "Read-only. Whether the controller reports UPnP as supported for the DS-Lite " +
					"or Map-E WAN connection types. Writes send back the value read.",
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// getUpnp fetches the live setting. It returns nil only on a controller or
// transport error (diagnostics are populated in that case).
func getUpnp(ctx context.Context, diags *diag.Diagnostics, r *upnpSettingResource, siteId string) *upnpReadVO {
	_, httpResp, callErr := r.client.ServiceAPI.GetUpnpSetting(ctx, r.omadacId, siteId).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading UPnP setting")
	if !ok {
		return nil
	}
	if env.HasError() {
		envelope.AddAPIError(diags, "reading UPnP setting", env.ErrorCode, env.Msg)
		return nil
	}
	var vo upnpReadVO
	if len(env.Result) > 0 && string(env.Result) != "null" {
		if err := json.Unmarshal(env.Result, &vo); err != nil {
			diags.AddError("Error reading UPnP setting", "Could not decode the setting: "+err.Error())
			return nil
		}
	}
	return &vo
}

// readUpnp refreshes the model in place from the live setting.
func readUpnp(ctx context.Context, diags *diag.Diagnostics, r *upnpSettingResource, model *upnpSettingResourceModel) bool {
	vo := getUpnp(ctx, diags, r, model.SiteId.ValueString())
	if vo == nil {
		return false
	}
	flattenUpnp(model, vo)
	return true
}

// writeUpnp PUTs the whole setting. It reads the live setting first so that
// fields the configuration leaves out are sent with their current value rather
// than cleared, then reads the result back into the plan.
func writeUpnp(ctx context.Context, diags *diag.Diagnostics, r *upnpSettingResource, plan *upnpSettingResourceModel, action string) bool {
	live := getUpnp(ctx, diags, r, plan.SiteId.ValueString())
	if live == nil {
		return false
	}
	body := expandUpnp(ctx, diags, *plan, live)
	if diags.HasError() {
		return false
	}

	_, httpResp, callErr := r.client.ServiceAPI.UpdateUpnpSetting(ctx, r.omadacId, plan.SiteId.ValueString()).
		UpnpSettingOpenApiVO(body).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, action)
	if !ok {
		return false
	}
	// A 2xx answer can still carry a controller error (for example -33474 on a
	// DS-Lite or Map-E WAN); that is a failed write, not a success.
	if env.HasError() {
		envelope.AddAPIError(diags, action, env.ErrorCode, env.Msg)
		return false
	}
	return readUpnp(ctx, diags, r, plan)
}

// Create applies the desired setting to the singleton (it always exists), then
// reads it back.
func (r *upnpSettingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan upnpSettingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !writeUpnp(ctx, &resp.Diagnostics, r, &plan, "creating UPnP setting") {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the live setting.
func (r *upnpSettingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state upnpSettingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readUpnp(ctx, &resp.Diagnostics, r, &state) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update sends the whole setting, then reads it back.
func (r *upnpSettingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state upnpSettingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteId = state.SiteId

	if !writeUpnp(ctx, &resp.Diagnostics, r, &plan, "updating UPnP setting") {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete only removes the setting from state. It is a singleton, and turning
// UPnP off (or on) as a side effect of a destroy would change the network's
// posture without anyone asking for it; declare `enable = false` instead.
func (r *upnpSettingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState imports the live setting. The import ID is `<site_id>`; the
// framework follows it with a Read that fills in the remaining attributes.
func (r *upnpSettingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" || strings.Contains(req.ID, "/") {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>` (the setting is a singleton per site), got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), req.ID)...)
}
