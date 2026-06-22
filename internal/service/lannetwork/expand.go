package lannetwork

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	// purposeInterface (1) is a gateway-terminated LAN network with an IP
	// interface (gatewaySubnet). This is the homelab shape and the resource
	// default. purposeVlan (0) is a VLAN-only network with no gateway interface.
	purposeInterface int32 = 1
	// vlanTypeSingle (0) is one VLAN tag per network, as opposed to a
	// multi-VLAN network (vlanType 1).
	vlanTypeSingle int32 = 0
	// errNetworkNotFound is the Omada Open API error code for a missing LAN
	// network. Read uses it to detect remote deletion.
	errNetworkNotFound int32 = -33503
)

// expandDhcpSettings converts the optional dhcp_settings block into the SDK
// type. Returns nil when the block is absent so the network is created without
// gateway DHCP.
func expandDhcpSettings(s *dhcpSettingsModel) *omada.DhcpSettings {
	if s == nil {
		return nil
	}

	return &omada.DhcpSettings{
		Enable:      s.Enable.ValueBoolPointer(),
		Dhcpns:      s.Dhcpns.ValueStringPointer(),
		Gateway:     s.Gateway.ValueStringPointer(),
		IpaddrStart: s.IpaddrStart.ValueStringPointer(),
		IpaddrEnd:   s.IpaddrEnd.ValueStringPointer(),
		Leasetime:   s.Leasetime.ValueInt32Pointer(),
		PriDns:      s.PriDns.ValueStringPointer(),
		SndDns:      s.SndDns.ValueStringPointer(),
	}
}

// expandInterfaceIds converts the Terraform list of interface IDs (gateway LAN
// port IDs) into the SDK slice. Returns nil when none are configured.
func expandInterfaceIds(ids []types.String) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id.IsNull() || id.IsUnknown() {
			continue
		}
		out = append(out, id.ValueString())
	}

	return out
}

// expandLanNetwork builds the SDK LAN network value sent on Create and Modify.
// vlanType is fixed to Single and the VLAN tag is taken from vlan_id.
func expandLanNetwork(plan lanNetworkResourceModel) omada.LanNetworkOpenApiVO {
	vlan := plan.VlanId.ValueInt32()
	vlanType := vlanTypeSingle
	purpose := purposeInterface
	if !plan.Purpose.IsNull() && !plan.Purpose.IsUnknown() {
		purpose = plan.Purpose.ValueInt32()
	}

	return omada.LanNetworkOpenApiVO{
		Name:            plan.Name.ValueString(),
		Purpose:         purpose,
		Vlan:            &vlan,
		VlanType:        &vlanType,
		GatewaySubnet:   plan.GatewaySubnet.ValueStringPointer(),
		InterfaceIds:    expandInterfaceIds(plan.InterfaceIds),
		Domain:          plan.Domain.ValueStringPointer(),
		IgmpSnoopEnable: plan.IgmpSnoopEnable.ValueBool(),
		DhcpSettingsVO:  expandDhcpSettings(plan.DhcpSettings),
	}
}
