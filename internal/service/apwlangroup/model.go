package apwlangroup

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// apWlanGroupClient is the SDK handle shared by the resource. It is populated
// from the provider Meta during Configure.
type apWlanGroupClient struct {
	client   *omada.APIClient
	omadacId string
}

// apWlanGroupResourceModel maps the omada_ap_wlan_group resource schema. It
// models which single WLAN group a managed access point (EAP) broadcasts.
//
// The Omada Open API models the AP↔WLAN-group relationship as 1:1 — every AP
// belongs to exactly one WLAN group — and exposes only a "switch this AP to a
// different group" write (PATCH /aps/{apMac}/wlan-group), never an
// unbind/membership toggle. This resource therefore behaves like a singleton
// keyed by (site_id, ap_mac): Create/Update switch the AP to the desired group
// (skipping the call when the AP is already on it, because the controller
// rejects a switch to the current group), Read fetches the AP's current group
// from the overview endpoint, and Delete is a no-op (an AP cannot be unbound;
// moving it "back" is expressed as a wlan_group_id change, not a destroy).
//
// The controller firmware targeted by this resource (Open API v1, e.g.
// 5.15.8.12 / 6.2.x) returns the current group on GET /aps/{apMac} under the
// literal JSON key "wlan group id". See docs/network/OMADA-TERRAFORM.md.
type apWlanGroupResourceModel struct {
	SiteId      types.String `tfsdk:"site_id"`
	ApMac       types.String `tfsdk:"ap_mac"`
	WlanGroupId types.String `tfsdk:"wlan_group_id"`
	ApName      types.String `tfsdk:"ap_name"`
}
