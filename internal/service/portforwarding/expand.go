package portforwarding

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func listToStrings(value types.List) []string {
	out := make([]string, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		v, ok := element.(types.String)
		if !ok || v.IsNull() || v.IsUnknown() {
			continue
		}
		out = append(out, v.ValueString())
	}
	return out
}

func mapToPortIps(value types.Map) []omada.PortIpOpenApiVO {
	out := make([]omada.PortIpOpenApiVO, 0, len(value.Elements()))
	for wanId, element := range value.Elements() {
		ip, ok := element.(types.String)
		if !ok || ip.IsNull() || ip.IsUnknown() {
			continue
		}
		wan := wanId
		address := ip.ValueString()
		out = append(out, omada.PortIpOpenApiVO{WanId: &wan, Ip: &address})
	}
	return out
}

// expandPortForwarding creates the request body for create/update. DMZ is
// always false by design. Omada's `from` discriminator is derived from the
// source allowlist: 0 means any source and 1 means limited addresses.
func expandPortForwarding(plan portForwardingResourceModel) omada.PortForwardingConfig {
	sources := listToStrings(plan.SourceAddresses)
	from := int32(0)
	if len(sources) > 0 {
		from = 1
	}

	externalPort := plan.ExternalPort.ValueString()
	forwardPort := plan.ForwardPort.ValueString()
	protocol := plan.Protocol.ValueInt32()

	return omada.PortForwardingConfig{
		DMZ:                false,
		ExternalPort:       &externalPort,
		ForwardIp:          plan.ForwardIp.ValueString(),
		ForwardPort:        &forwardPort,
		From:               from,
		InterfaceWanPortId: listToStrings(plan.WanPortIds),
		LimitedAddresses:   sources,
		Name:               plan.Name.ValueString(),
		Protocol:           &protocol,
		Status:             plan.Status.ValueBool(),
		VirtualWanId:       listToStrings(plan.VirtualWanIds),
		WanIps:             mapToPortIps(plan.WanIps),
	}
}
