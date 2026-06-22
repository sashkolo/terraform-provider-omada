package lannetwork

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// lanNetworkClient is the SDK handle shared by the resource. It is populated
// from the provider Meta during Configure.
type lanNetworkClient struct {
	client   *omada.APIClient
	omadacId string
}

// dhcpSettingsModel maps the gateway DHCP settings attached to a LAN network.
// Mirrors the SDK's DhcpSettings type, which is what the controller returns and
// accepts for both read and write on the v1 LAN-network surface.
type dhcpSettingsModel struct {
	Enable      types.Bool   `tfsdk:"enable"`
	Dhcpns      types.String `tfsdk:"dhcpns"`
	Gateway     types.String `tfsdk:"gateway"`
	IpaddrStart types.String `tfsdk:"ipaddr_start"`
	IpaddrEnd   types.String `tfsdk:"ipaddr_end"`
	Leasetime   types.Int32  `tfsdk:"leasetime"`
	PriDns      types.String `tfsdk:"pri_dns"`
	SndDns      types.String `tfsdk:"snd_dns"`
}

// lanNetworkResourceModel maps the omada_lan_network resource schema. It models
// a gateway-served LAN network: the Omada gateway terminates the VLAN (purpose
// "interface"), owns the gateway IP, and optionally serves DHCP/DNS for it.
//
// The controller firmware targeted by this resource (Open API v1, e.g.
// 5.15.8.12) exposes the LAN-network surface at /openapi/v1/.../lan-networks.
// Newer controllers add a two-step "confirm" flow at /networks/confirm; this
// resource deliberately uses the v1 surface because that is what the deployed
// controller implements.
type lanNetworkResourceModel struct {
	NetworkId       types.String       `tfsdk:"network_id"`
	SiteId          types.String       `tfsdk:"site_id"`
	Name            types.String       `tfsdk:"name"`
	VlanId          types.Int32        `tfsdk:"vlan_id"`
	Purpose         types.Int32        `tfsdk:"purpose"`
	GatewaySubnet   types.String       `tfsdk:"gateway_subnet"`
	Domain          types.String       `tfsdk:"domain"`
	IgmpSnoopEnable types.Bool         `tfsdk:"igmp_snoop_enable"`
	DhcpSettings    *dhcpSettingsModel `tfsdk:"dhcp_settings"`
}
