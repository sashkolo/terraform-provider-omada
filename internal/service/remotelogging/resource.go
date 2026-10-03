package remotelogging

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
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                   = &remoteLoggingResource{}
	_ resource.ResourceWithConfigure      = &remoteLoggingResource{}
	_ resource.ResourceWithImportState    = &remoteLoggingResource{}
	_ resource.ResourceWithValidateConfig = &remoteLoggingResource{}
)

// NewResource is a helper function to simplify the provider implementation.
func NewResource() resource.Resource {
	return &remoteLoggingResource{}
}

// remoteLoggingResource is the resource implementation.
type remoteLoggingResource struct {
	remoteLoggingClient
}

// Configure adds the provider configured client to the resource.
func (r *remoteLoggingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *remoteLoggingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_remote_logging_setting"
}

// Schema defines the schema for the resource.
func (r *remoteLoggingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the site's remote logging (syslog) target, to which the site's devices " +
			"send their logs. This is a singleton per site: the object always exists, so import it " +
			"(`<site_id>`) to adopt the live setting before managing it. Create/Update PATCH the " +
			"target; Delete is a no-op (the setting is never reset on destroy).",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				Description: "Site ID the setting belongs to; the import target. Changing this forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enable": schema.BoolAttribute{
				Description: "Whether remote logging is enabled.",
				Required:    true,
			},
			"host": schema.StringAttribute{
				Description: "IP address of the remote syslog server.",
				Required:    true,
			},
			"port": schema.Int32Attribute{
				Description: "Port of the remote syslog server, 1-65535.",
				Required:    true,
			},
			"more_client_log": schema.BoolAttribute{
				Description: "Whether client logs are included. When unset, the live value is kept.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ValidateConfig rejects a port outside 1-65535 before anything is sent.
func (r *remoteLoggingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg remoteLoggingResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Port.IsNull() || cfg.Port.IsUnknown() {
		return
	}
	if p := cfg.Port.ValueInt32(); p < 1 || p > 65535 {
		resp.Diagnostics.AddAttributeError(path.Root("port"), "Invalid port",
			fmt.Sprintf("port must be within 1-65535, got %d.", p))
	}
}

// readRemoteLogging fetches the singleton and refreshes the model in place. It
// returns false only on a controller or transport error.
func readRemoteLogging(ctx context.Context, diags *diag.Diagnostics, r *remoteLoggingResource, model *remoteLoggingResourceModel) bool {
	_, httpResp, callErr := r.client.SiteConfigurationAPI.GetRemoteLoggingSetting(ctx, r.omadacId, model.SiteId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading remote logging setting")
	if !ok {
		return false
	}
	if env.HasError() {
		envelope.AddAPIError(diags, "reading remote logging setting", env.ErrorCode, env.Msg)
		return false
	}
	var vo remoteLoggingReadVO
	if len(env.Result) > 0 && string(env.Result) != "null" {
		if err := json.Unmarshal(env.Result, &vo); err != nil {
			diags.AddError("Error reading remote logging setting", "Could not decode the setting: "+err.Error())
			return false
		}
	}
	flattenRemoteLogging(model, &vo)
	return true
}

// writeRemoteLogging PATCHes the target from the plan.
func writeRemoteLogging(ctx context.Context, diags *diag.Diagnostics, r *remoteLoggingResource, plan remoteLoggingResourceModel, action string) bool {
	_, httpResp, callErr := r.client.SiteConfigurationAPI.UpdateRemoteLoggingSetting(ctx, r.omadacId, plan.SiteId.ValueString()).
		SiteRemoteLoggingSetting(expandRemoteLogging(plan)).Execute()
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

// Create applies the desired target to the singleton (it always exists), then
// reads it back.
func (r *remoteLoggingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan remoteLoggingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !writeRemoteLogging(ctx, &resp.Diagnostics, r, plan, "creating remote logging setting") {
		return
	}
	if !readRemoteLogging(ctx, &resp.Diagnostics, r, &plan) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the live setting.
func (r *remoteLoggingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state remoteLoggingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !readRemoteLogging(ctx, &resp.Diagnostics, r, &state) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update PATCHes the target, then reads it back.
func (r *remoteLoggingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state remoteLoggingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteId = state.SiteId

	if !writeRemoteLogging(ctx, &resp.Diagnostics, r, plan, "updating remote logging setting") {
		return
	}
	if !readRemoteLogging(ctx, &resp.Diagnostics, r, &plan) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete is a no-op: the setting is a singleton, and disabling or clearing it on
// destroy would silently stop the site's logs. Removing it from state is the
// correct end state.
func (r *remoteLoggingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState imports the live setting. The import ID is `<site_id>`; the
// framework follows it with a Read that fills in the remaining attributes.
func (r *remoteLoggingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" || strings.Contains(req.ID, "/") {
		resp.Diagnostics.AddError(
			"Unexpected Import ID",
			fmt.Sprintf("Expected import ID in the form `<site_id>` (the setting is a singleton per site), got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), req.ID)...)
}
