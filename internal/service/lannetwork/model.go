package lannetwork

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// lanNetworkClient is the SDK handle shared by the resource (and, later, any
// data source in this package). It is populated from the provider Meta during
// Configure.
type lanNetworkClient struct {
	client   *omada.APIClient
	omadacId string
}

// dhcpRangeModel maps a single DHCP address-pool range.
type dhcpRangeModel struct {
	StartIp types.String `tfsdk:"start"`
	EndIp   types.String `tfsdk:"end"`
}

// dhcpServerModel maps the gateway-served DHCP server configuration attached to
// a LAN network. Mirrors the SDK's OswDhcpServerOpenApiVO, which is the same
// type used for both the write (Create/Modify) and the read (Get) payloads.
type dhcpServerModel struct {
	Gateway     types.String     `tfsdk:"gateway"`
	Ip          types.String     `tfsdk:"ip"`
	Netmask     types.String     `tfsdk:"netmask"`
	Leasetime   types.Int32      `tfsdk:"leasetime"`
	PriDns      types.String     `tfsdk:"pri_dns"`
	SndDns      types.String     `tfsdk:"snd_dns"`
	IpRangePool []dhcpRangeModel `tfsdk:"ip_range_pool"`
}

// lanNetworkResourceModel maps the omada_lan_network resource schema. It models
// a gateway-served, single-VLAN LAN network (deviceType=1, vlanType=0), which is
// the common homelab shape: the Omada gateway terminates the VLAN, owns the
// gateway IP, and serves DHCP/DNS for it.
type lanNetworkResourceModel struct {
	NetworkId       types.String     `tfsdk:"network_id"`
	SiteId          types.String     `tfsdk:"site_id"`
	Name            types.String     `tfsdk:"name"`
	VlanId          types.Int32      `tfsdk:"vlan_id"`
	GatewaySubnet   types.String     `tfsdk:"gateway_subnet"`
	Domain          types.String     `tfsdk:"domain"`
	IgmpSnoopEnable types.Bool       `tfsdk:"igmp_snoop_enable"`
	DeviceType      types.Int32      `tfsdk:"device_type"`
	DhcpServer      *dhcpServerModel `tfsdk:"dhcp_server"`
}
