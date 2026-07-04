package ipportgroup

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// groupBodyTypePort is the IP-Port group discriminator in the create/modify
// body. It mirrors the groupTypePort path segment (URL carries the type as a
// string, the body as an integer).
const groupBodyTypePort int32 = 1

// defaultPortType is the port mode applied when port_type is left unset: 0 is
// port-list mode (the common case: an explicit list of service ports).
const defaultPortType int32 = 0

// nonEmptyStringPointer returns p only when it points at a non-empty string,
// otherwise nil, so an omitted optional stays out of the request body.
func nonEmptyStringPointer(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	return p
}

// expandIpList converts the ip_list nested block into the SDK slice. Always
// returns a non-nil slice so it serializes as [] when empty.
func expandIpList(in []ipSubnetModel) []omada.IPSubnetsOpenApiVO {
	out := make([]omada.IPSubnetsOpenApiVO, 0, len(in))
	for _, e := range in {
		out = append(out, omada.IPSubnetsOpenApiVO{
			Ip:          e.Ip.ValueString(),
			Mask:        e.Mask.ValueInt32(),
			Description: nonEmptyStringPointer(e.Description.ValueStringPointer()),
		})
	}
	return out
}

// expandPortList converts the port_list attribute into the SDK slice. Tolerates
// null/unknown elements. Returns nil when empty so it is omitted from the body
// (port-mask mode carries no port list).
func expandPortList(in []types.String) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v.IsNull() || v.IsUnknown() {
			continue
		}
		out = append(out, v.ValueString())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// expandPortMaskList converts the port_mask_list nested block into the SDK
// slice. Returns nil when empty so it is omitted from the body.
func expandPortMaskList(in []portMaskModel) []omada.PortMaskOpenApiVO {
	if len(in) == 0 {
		return nil
	}
	out := make([]omada.PortMaskOpenApiVO, 0, len(in))
	for _, e := range in {
		out = append(out, omada.PortMaskOpenApiVO{
			Port: e.Port.ValueInt32(),
			Mask: e.Mask.ValueString(),
		})
	}
	return out
}

// expandGroup builds the SDK group-profile config value sent on Create and
// Modify for an IP-Port group (type 1). port_type defaults to port-list mode.
func expandGroup(plan ipPortGroupResourceModel) omada.CreateGroupOpenApiVO {
	portType := defaultPortType
	if !plan.PortType.IsNull() && !plan.PortType.IsUnknown() {
		portType = plan.PortType.ValueInt32()
	}

	return omada.CreateGroupOpenApiVO{
		Name:         plan.Name.ValueString(),
		Description:  nonEmptyStringPointer(plan.Description.ValueStringPointer()),
		IpList:       expandIpList(plan.IpList),
		PortType:     &portType,
		PortList:     expandPortList(plan.PortList),
		PortMaskList: expandPortMaskList(plan.PortMaskList),
		Type:         groupBodyTypePort,
	}
}
