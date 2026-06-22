package lannetwork

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenDhcpRanges converts the SDK DHCP range list into the Terraform block
// list.
func flattenDhcpRanges(r []omada.OswDhcpServerRangeOpenApiVO) []dhcpRangeModel {
	out := make([]dhcpRangeModel, 0, len(r))
	for _, r := range r {
		out = append(out, dhcpRangeModel{
			StartIp: types.StringValue(r.StartIp),
			EndIp:   types.StringValue(r.EndIp),
		})
	}

	return out
}

// flattenDhcpServer converts the SDK DHCP server value into the Terraform block.
// Returns nil when the API reports no gateway-served DHCP server, which keeps
// the block absent from state (and the plan diff clean) for networks without
// one.
func flattenDhcpServer(s *omada.OswDhcpServerOpenApiVO) *dhcpServerModel {
	if s == nil {
		return nil
	}

	return &dhcpServerModel{
		Gateway:     types.StringPointerValue(s.Gateway),
		Ip:          types.StringPointerValue(&s.Ip),
		Netmask:     types.StringPointerValue(&s.Netmask),
		Leasetime:   types.Int32PointerValue(&s.Leasetime),
		PriDns:      types.StringPointerValue(s.PriDns),
		SndDns:      types.StringPointerValue(s.SndDns),
		IpRangePool: flattenDhcpRanges(s.IpRangePool),
	}
}

// flattenLanNetwork overwrites the resource model from the SDK read response.
// network_id and site_id are preserved from the prior state because Get is
// keyed by them; the remaining fields are refreshed from the controller.
func flattenLanNetwork(m *lanNetworkResourceModel, r *omada.LanNetworkQueryOpenApiV3VO) {
	if r == nil {
		return
	}

	m.NetworkId = types.StringPointerValue(r.Id)
	m.Name = types.StringPointerValue(&r.Name)
	m.VlanId = types.Int32PointerValue(r.Vlan)
	m.GatewaySubnet = types.StringPointerValue(r.GatewaySubnet)
	m.Domain = types.StringPointerValue(r.Domain)
	m.IgmpSnoopEnable = types.BoolValue(r.IgmpSnoopEnable)
	m.DeviceType = types.Int32Value(r.DeviceType)
	m.DhcpServer = flattenDhcpServer(r.DhcpServer)
}
