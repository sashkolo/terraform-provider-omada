package lannetwork

import (
	"fmt"
	"strings"

	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// carryUnmodeled copies the live values of the settings this resource doesn't
// model into an update body, so an edit to a modeled attribute can't reset
// them. The Open API's PATCH semantics for omitted fields are
// undocumented, so nothing is left to chance:
//
//   - simple settings the SDK can send (L2 relay, isolation, MLD snooping,
//     all-LAN, application, DHCP next-server and options 60/66/138) are sent
//     with their live values;
//   - settings it can't send faithfully (DHCP and DHCPv6 guard, IPv6, custom
//     DHCP options; the SDK even misspells the IPv6 key) make the update fail
//     when they are on, rather than risk resetting them. They are off on a
//     typical network, so an ordinary edit is unaffected.
//
// It returns false, with an error diagnostic, when the update must not run.
func carryUnmodeled(body *omada.LanNetworkOpenApiVO, live *lanNetworkReadRow, diags *diag.Diagnostics) bool {
	body.AllLan = live.AllLan
	body.Application = live.Application
	body.DhcpL2RelayEnable = live.DhcpL2RelayEnable
	body.Isolation = live.Isolation
	body.MldSnoopEnable = live.MldSnoopEnable

	if body.DhcpSettingsVO != nil && live.DhcpSettingsVO != nil {
		body.DhcpSettingsVO.DhcpNextServer = live.DhcpSettingsVO.DhcpNextServer
		body.DhcpSettingsVO.Option60 = live.DhcpSettingsVO.Option60
		body.DhcpSettingsVO.Option66 = live.DhcpSettingsVO.Option66
		body.DhcpSettingsVO.Option138 = live.DhcpSettingsVO.Option138
	}

	var on []string
	if live.DhcpGuard != nil && live.DhcpGuard.Enable != nil && *live.DhcpGuard.Enable {
		on = append(on, "DHCP guard")
	}
	if live.Dhcpv6Guard != nil && live.Dhcpv6Guard.Enable != nil && *live.Dhcpv6Guard.Enable {
		on = append(on, "DHCPv6 guard")
	}
	if live.LanNetworkIpv6Config != nil && live.LanNetworkIpv6Config.Enable != nil && *live.LanNetworkIpv6Config.Enable != 0 {
		on = append(on, "IPv6")
	}
	if live.DhcpSettingsVO != nil && len(live.DhcpSettingsVO.Options) > 0 {
		on = append(on, "custom DHCP options")
	}
	if len(on) > 0 {
		diags.AddError("Error updating LAN network", fmt.Sprintf(
			"LAN network %q has %s turned on in the controller. This resource doesn't model %s, and an update "+
				"might reset it, so the update was not sent. Change the network in the controller UI, or turn "+
				"the setting off there first.", live.Name, strings.Join(on, ", "), map[bool]string{true: "them", false: "it"}[len(on) > 1]))
		return false
	}
	return true
}
