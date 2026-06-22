package lannetwork

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenDhcpRead converts the lenient local DHCP read view into the Terraform
// block. Returns nil when the controller reports no DHCP settings.
func flattenDhcpRead(s *dhcpReadVO) *dhcpSettingsModel {
	if s == nil {
		return nil
	}

	return &dhcpSettingsModel{
		Enable:      types.BoolPointerValue(s.Enable),
		Dhcpns:      types.StringPointerValue(s.Dhcpns),
		Gateway:     types.StringPointerValue(s.Gateway),
		IpaddrStart: types.StringPointerValue(s.IpaddrStart),
		IpaddrEnd:   types.StringPointerValue(s.IpaddrEnd),
		Leasetime:   types.Int32PointerValue(s.Leasetime),
		PriDns:      types.StringPointerValue(s.PriDns),
		SndDns:      types.StringPointerValue(s.SndDns),
	}
}

// flattenInterfaceIds converts the controller's interface ID list into the
// Terraform list. Always returns a non-nil slice so it serializes as [] when
// empty.
func flattenInterfaceIds(ids []string) []types.String {
	out := make([]types.String, 0, len(ids))
	for _, id := range ids {
		out = append(out, types.StringValue(id))
	}

	return out
}

// flattenLanNetworkRead overwrites the resource model from a lenient read row.
// network_id and site_id are preserved from the prior state (Read is keyed by
// them); the remaining fields are refreshed from the controller.
func flattenLanNetworkRead(m *lanNetworkResourceModel, r *lanNetworkReadRow) {
	if r == nil {
		return
	}

	m.NetworkId = types.StringPointerValue(r.Id)
	m.Name = types.StringValue(r.Name)
	m.VlanId = types.Int32PointerValue(r.Vlan)
	m.Purpose = types.Int32Value(r.Purpose)
	m.GatewaySubnet = types.StringPointerValue(r.GatewaySubnet)
	m.Domain = types.StringPointerValue(r.Domain)
	m.IgmpSnoopEnable = types.BoolValue(r.IgmpSnoopEnable)
	m.InterfaceIds = flattenInterfaceIds(r.InterfaceIds)
	m.DhcpSettings = flattenDhcpRead(r.DhcpSettingsVO)
}
