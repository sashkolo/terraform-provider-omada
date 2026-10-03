package lannetwork

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenDhcpForModel keeps an unset dhcp_settings unset while the gateway
// serves no DHCP. The controller reports a DHCP block with enable = false for a
// network created without one (and the UI's LAN form fills in defaults such as
// dhcpns and leasetime), so reading that back into an unset attribute failed
// the create with "inconsistent result after apply". Found by a live write
// proof. DHCP turned on in the UI still shows up as drift.
func flattenDhcpForModel(s *dhcpReadVO, prior *dhcpSettingsModel) *dhcpSettingsModel {
	if prior == nil && (s == nil || s.Enable == nil || !*s.Enable) {
		return nil
	}
	return flattenDhcpRead(s)
}

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
// Terraform list. An empty live list stays null when the prior value was null
// (the attribute unset), so an unset list doesn't read back as [] and fail the
// apply with an inconsistent result; otherwise it serializes as [].
func flattenInterfaceIds(ids []string, prior []types.String) []types.String {
	if len(ids) == 0 && prior == nil {
		return nil
	}
	out := make([]types.String, 0, len(ids))
	for _, id := range ids {
		out = append(out, types.StringValue(id))
	}

	return out
}

// defaultNameSuffix is what the controller appends, for display, to the stored
// name of the site's default network in the LAN-network list.
const defaultNameSuffix = "(Default)"

// flattenName returns the stored network name. The list reports the default
// network as "<name>(Default)", but an update stores the name it is sent
// verbatim, so reading the suffix into state renamed the network on its next
// update ("Management" became "Management(Default)") and then failed the apply
// with an inconsistent result. Found by a live DHCP-pool update.
func flattenName(r *lanNetworkReadRow) string {
	if r.Primary {
		return strings.TrimSuffix(r.Name, defaultNameSuffix)
	}
	return r.Name
}

// flattenLanNetworkRead overwrites the resource model from a lenient read row.
// network_id and site_id are preserved from the prior state (Read is keyed by
// them); the remaining fields are refreshed from the controller.
func flattenLanNetworkRead(m *lanNetworkResourceModel, r *lanNetworkReadRow) {
	if r == nil {
		return
	}

	m.NetworkId = types.StringPointerValue(r.Id)
	m.Name = types.StringValue(flattenName(r))
	m.VlanId = types.Int32PointerValue(r.Vlan)
	m.Purpose = types.Int32Value(r.Purpose)
	m.GatewaySubnet = types.StringPointerValue(r.GatewaySubnet)
	m.Domain = types.StringPointerValue(r.Domain)
	m.IgmpSnoopEnable = types.BoolValue(r.IgmpSnoopEnable)
	m.InterfaceIds = flattenInterfaceIds(r.InterfaceIds, m.InterfaceIds)
	m.DhcpSettings = flattenDhcpForModel(r.DhcpSettingsVO, m.DhcpSettings)
}
