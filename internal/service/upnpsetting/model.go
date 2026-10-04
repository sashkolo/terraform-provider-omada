package upnpsetting

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// upnpClient is the SDK handle shared by the resource. It is populated from the
// provider Meta during Configure.
type upnpClient struct {
	client   *omada.APIClient
	omadacId string
}

// upnpSettingResourceModel maps the omada_upnp_setting resource schema: the
// site's gateway UPnP setting. This is a singleton per site (the object always
// exists); the resource imports as `<site_id>`, Create/Update PUT the whole
// object, Read GETs it, and Delete only removes it from state.
type upnpSettingResourceModel struct {
	SiteId                 types.String `tfsdk:"site_id"`
	Enable                 types.Bool   `tfsdk:"enable"`
	NetworkIds             types.Set    `tfsdk:"network_ids"`
	WanPortIds             types.Set    `tfsdk:"wan_port_ids"`
	SupportByDsLiteAndMapE types.Bool   `tfsdk:"support_by_ds_lite_and_map_e"`
}
