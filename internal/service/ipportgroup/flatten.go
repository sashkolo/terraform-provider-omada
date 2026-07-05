package ipportgroup

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// stringPtrOrNull maps a controller string pointer to a Terraform value,
// collapsing both nil and "" to null so an unset optional stays stable.
func stringPtrOrNull(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// flattenIpList converts the controller's ipList into the nested block. Returns
// nil for an empty input so an absent list stays null in state.
func flattenIpList(in []ipSubnetReadVO) []ipSubnetModel {
	if len(in) == 0 {
		return nil
	}
	out := make([]ipSubnetModel, 0, len(in))
	for _, e := range in {
		out = append(out, ipSubnetModel{
			Ip:          types.StringValue(e.Ip),
			Mask:        types.Int32Value(e.Mask),
			Description: stringPtrOrNull(e.Description),
		})
	}
	return out
}

// flattenPortList converts the controller's portList into the Terraform slice.
// Returns nil for an empty input so an absent list stays null.
func flattenPortList(in []string) []types.String {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.String, 0, len(in))
	for _, v := range in {
		out = append(out, types.StringValue(v))
	}
	return out
}

// flattenPortMaskList converts the controller's portMaskList into the nested
// block. Returns nil for an empty input.
func flattenPortMaskList(in []portMaskReadVO) []portMaskModel {
	if len(in) == 0 {
		return nil
	}
	out := make([]portMaskModel, 0, len(in))
	for _, e := range in {
		out = append(out, portMaskModel{
			Port: types.Int32Value(e.Port),
			Mask: types.StringValue(e.Mask),
		})
	}
	return out
}

// flattenGroupRead overwrites the resource model from a lenient read row.
// site_id is preserved from the prior state; the remaining fields are refreshed
// from the controller.
func flattenGroupRead(m *ipPortGroupResourceModel, r *groupReadRow) {
	if r == nil {
		return
	}
	m.GroupId = types.StringPointerValue(r.GroupId)
	m.Name = types.StringValue(r.Name)
	m.Description = stringPtrOrNull(r.Description)
	m.IpList = flattenIpList(r.IpList)
	m.PortType = types.Int32PointerValue(r.PortType)
	m.PortList = flattenPortList(r.PortList)
	m.PortMaskList = flattenPortMaskList(r.PortMaskList)
}
