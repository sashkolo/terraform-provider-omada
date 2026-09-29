package switchport

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// switchPortClient is the SDK handle shared by the resource. It is populated
// from the provider Meta during Configure.
type switchPortClient struct {
	client   *omada.APIClient
	omadacId string
}

// switchPortResourceModel maps the omada_switch_port resource schema. It models
// the per-port assignment on a managed Omada switch: which reusable port
// profile the port uses (and therefore its VLAN posture), plus port name, PoE
// mode, and admin state.
//
// A physical switch port always exists, so this resource behaves like a
// singleton keyed by (site_id, switch_mac, port): Create/Update apply the
// desired settings via the per-port Modify endpoint, Read fetches the port from
// the switch overview, and Delete is a no-op (a port cannot be removed).
//
// The controller firmware targeted by this resource (Open API v1, e.g.
// 5.15.8.12) exposes the readable per-port fields via GET /switches/{mac}
// (portList) and the write via PATCH /switches/{mac}/ports/{port}. That
// firmware does not expose the per-port VLAN override contents on read; the
// port's VLAN posture is therefore governed by (and drift-detected through) the
// referenced omada_switch_port_profile. See docs/network/OMADA-TERRAFORM.md.
type switchPortResourceModel struct {
	SiteId                types.String `tfsdk:"site_id"`
	SwitchMac             types.String `tfsdk:"switch_mac"`
	Port                  types.Int32  `tfsdk:"port"`
	Name                  types.String `tfsdk:"name"`
	ProfileId             types.String `tfsdk:"profile_id"`
	ProfileName           types.String `tfsdk:"profile_name"`
	ProfileOverrideEnable types.Bool   `tfsdk:"profile_override_enable"`
	Poe                   types.Int32  `tfsdk:"poe"`
	Disabled              types.Bool   `tfsdk:"disabled"`
	LagPort               types.Bool   `tfsdk:"lag_port"`
	AllowTrunkReassign    types.Bool   `tfsdk:"allow_trunk_reassign"`
	AllowLagMember        types.Bool   `tfsdk:"allow_lag_member"`
}
