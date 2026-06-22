package lannetwork

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenDhcpSettings converts the SDK DHCP settings value into the Terraform
// block. Returns nil when the API reports no DHCP settings, which keeps the
// block absent from state (and the plan diff clean) for networks without it.
func flattenDhcpSettings(s *omada.DhcpSettings) *dhcpSettingsModel {
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

// flattenLanNetwork overwrites the resource model from the SDK read row.
// network_id and site_id are preserved from the prior state (Read is keyed by
// them); the remaining fields are refreshed from the controller.
func flattenLanNetwork(m *lanNetworkResourceModel, r *omada.LanNetworkQueryOpenApiVO) {
	if r == nil {
		return
	}

	m.NetworkId = types.StringPointerValue(r.Id)
	m.Name = types.StringPointerValue(&r.Name)
	m.VlanId = types.Int32PointerValue(r.Vlan)
	m.Purpose = types.Int32Value(r.Purpose)
	m.GatewaySubnet = types.StringPointerValue(r.GatewaySubnet)
	m.Domain = types.StringPointerValue(r.Domain)
	m.IgmpSnoopEnable = types.BoolValue(r.IgmpSnoopEnable)
	m.DhcpSettings = flattenDhcpSettings(r.DhcpSettingsVO)
}
