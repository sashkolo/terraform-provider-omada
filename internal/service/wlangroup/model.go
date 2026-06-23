package wlangroup

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// wlanGroupClient is the SDK handle shared by the resource. It is populated
// from the provider Meta during Configure.
type wlanGroupClient struct {
	client   *omada.APIClient
	omadacId string
}

// wlanGroupResourceModel maps the omada_wlan_group resource schema. A WLAN
// group is a named collection of SSIDs that the controller binds to APs. This
// resource manages the group lifecycle (create/rename/delete); the SSIDs it
// contains are managed by omada_ssid.
//
// The controller firmware targeted by this resource (Open API v1, e.g.
// 5.15.8.12) does not implement a single-group GET on /wlans/{wlanId} (it
// returns HTTP 405), so Read fetches the group list and selects by wlan_id.
type wlanGroupResourceModel struct {
	WlanId types.String `tfsdk:"wlan_group_id"`
	SiteId types.String `tfsdk:"site_id"`
	Name   types.String `tfsdk:"name"`
	// Primary is computed: the controller marks exactly one group per site as
	// primary (the "Default" group). This resource never creates or flips a
	// primary group; it only reports the flag on read.
	Primary types.Bool `tfsdk:"primary"`
}
