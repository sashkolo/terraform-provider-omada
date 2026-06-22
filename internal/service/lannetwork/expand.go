package lannetwork

import (
	"github.com/Tohaker/omada-go-sdk/omada"
)

const (
	// deviceTypeGateway means the Omada gateway terminates the VLAN and serves
	// DHCP. It is the only device type this resource currently exercises.
	deviceTypeGateway int32 = 1
	// vlanTypeSingle means a single VLAN tag per network (vlanType=0), as
	// opposed to a multi-VLAN network (vlanType=1).
	vlanTypeSingle int32 = 0
	// errNetworkNotFound is the Omada Open API error code returned when a LAN
	// network does not exist. Read uses it to detect remote deletion.
	errNetworkNotFound int32 = -33503
)

// expandDhcpRanges converts the Terraform list of DHCP range blocks into the SDK
// type. It always returns a non-nil slice so the JSON serializes as [] rather
// than null when the user omits the pool.
func expandDhcpRanges(ranges []dhcpRangeModel) []omada.OswDhcpServerRangeOpenApiVO {
	out := make([]omada.OswDhcpServerRangeOpenApiVO, 0, len(ranges))
	for _, r := range ranges {
		out = append(out, omada.OswDhcpServerRangeOpenApiVO{
			StartIp: r.StartIp.ValueString(),
			EndIp:   r.EndIp.ValueString(),
		})
	}

	return out
}

// expandDhcpServer converts the optional dhcp_server block into the SDK type.
// Returns nil when the block is absent so the network is created without a
// gateway-served DHCP server.
func expandDhcpServer(s *dhcpServerModel) *omada.OswDhcpServerOpenApiVO {
	if s == nil {
		return nil
	}

	return &omada.OswDhcpServerOpenApiVO{
		Gateway:     s.Gateway.ValueStringPointer(),
		Ip:          s.Ip.ValueString(),
		Netmask:     s.Netmask.ValueString(),
		Leasetime:   s.Leasetime.ValueInt32(),
		PriDns:      s.PriDns.ValueStringPointer(),
		SndDns:      s.SndDns.ValueStringPointer(),
		IpRangePool: expandDhcpRanges(s.IpRangePool),
	}
}

// expandLanNetwork builds the SDK LAN network value sent on Create and Modify.
// vlanType is fixed to Single and the VLAN tag is taken from vlan_id.
func expandLanNetwork(plan lanNetworkResourceModel) omada.LanNetworkOpenApiV3VO {
	vlan := plan.VlanId.ValueInt32()
	vlanType := vlanTypeSingle
	deviceType := deviceTypeGateway
	if !plan.DeviceType.IsNull() && !plan.DeviceType.IsUnknown() {
		deviceType = plan.DeviceType.ValueInt32()
	}

	return omada.LanNetworkOpenApiV3VO{
		Name:            plan.Name.ValueString(),
		Vlan:            &vlan,
		VlanType:        &vlanType,
		GatewaySubnet:   plan.GatewaySubnet.ValueStringPointer(),
		Domain:          plan.Domain.ValueStringPointer(),
		IgmpSnoopEnable: plan.IgmpSnoopEnable.ValueBool(),
		DeviceType:      deviceType,
		DhcpServer:      expandDhcpServer(plan.DhcpServer),
	}
}
