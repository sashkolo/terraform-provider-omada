package aclconfigmode

import (
	"context"
	"encoding/json"
	"fmt"
	"terraform-provider-omada/internal/client"
	"terraform-provider-omada/internal/envelope"

	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &aclConfigModeDataSource{}
	_ datasource.DataSourceWithConfigure = &aclConfigModeDataSource{}
)

// NewDataSource is a helper function to simplify the provider implementation.
func NewDataSource() datasource.DataSource {
	return &aclConfigModeDataSource{}
}

// aclConfigModeDataSource reads the site's gateway ACL configuration mode.
type aclConfigModeDataSource struct {
	client   *omada.APIClient
	omadacId string
}

// aclConfigModeModel maps the data source schema.
type aclConfigModeModel struct {
	SiteId types.String `tfsdk:"site_id"`
	Mode   types.Int32  `tfsdk:"mode"`
	Custom types.Bool   `tfsdk:"custom"`
}

// modeVO is the lenient result of GET /acls/osg-config-mode.
type modeVO struct {
	Mode *int32 `json:"mode"`
}

// Configure adds the provider configured client to the data source.
func (d *aclConfigModeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*client.Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Meta, got %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = data.Client
	d.omadacId = data.OmadacId
}

// Metadata returns the data source type name.
func (d *aclConfigModeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gateway_acl_config_mode"
}

// Schema defines the schema for the data source.
func (d *aclConfigModeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads the site's gateway ACL configuration mode: `0` configures gateway ACLs " +
			"through profiles (the `osg-acls` that `omada_acl` manages), `1` through custom ACLs. " +
			"If the mode is switched in the controller UI, the gateway may stop enforcing the ACLs " +
			"this provider manages. Guard against that with a `postcondition` on this data source, " +
			"which fails the plan; a `check` block would only warn.",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				Description: "Site ID to read.",
				Required:    true,
			},
			"mode": schema.Int32Attribute{
				Description: "Gateway ACL configuration mode: `0` through profiles, `1` custom.",
				Computed:    true,
			},
			"custom": schema.BoolAttribute{
				Description: "Whether the gateway uses custom ACLs (`mode = 1`).",
				Computed:    true,
			},
		},
	}
}

// Read fetches the mode.
func (d *aclConfigModeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg aclConfigModeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, callErr := d.client.ACLAPI.GetAclConfigTypeSetting(ctx, d.omadacId, cfg.SiteId.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, &resp.Diagnostics, "reading gateway ACL config mode")
	if !ok {
		return
	}
	if env.HasError() {
		envelope.AddAPIError(&resp.Diagnostics, "reading gateway ACL config mode", env.ErrorCode, env.Msg)
		return
	}
	var vo modeVO
	if err := json.Unmarshal(env.Result, &vo); err != nil || vo.Mode == nil {
		msg := "the controller returned no mode"
		if err != nil {
			msg = err.Error()
		}
		resp.Diagnostics.AddError("Error reading gateway ACL config mode", "Could not decode the mode: "+msg)
		return
	}

	cfg.Mode = types.Int32Value(*vo.Mode)
	cfg.Custom = types.BoolValue(*vo.Mode == 1)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
