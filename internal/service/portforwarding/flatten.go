package portforwarding

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func stringsToList(values []string) types.List {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	result, _ := types.ListValue(types.StringType, elements)
	return result
}

func portIpsToMap(values []portIpRead) types.Map {
	elements := make(map[string]attr.Value, len(values))
	for _, value := range values {
		if value.WanId == nil || *value.WanId == "" || value.Ip == nil || *value.Ip == "" {
			continue
		}
		elements[*value.WanId] = types.StringValue(*value.Ip)
	}
	result, _ := types.MapValue(types.StringType, elements)
	return result
}

func flattenPortForwarding(model *portForwardingResourceModel, row *portForwardingReadRow) {
	if row == nil {
		return
	}
	model.PortForwardingId = types.StringPointerValue(row.Id)
	model.Name = types.StringValue(row.Name)
	model.Status = types.BoolValue(row.Status)
	model.ExternalPort = types.StringPointerValue(row.ExternalPort)
	model.ForwardIp = types.StringValue(row.ForwardIp)
	model.ForwardPort = types.StringPointerValue(row.ForwardPort)
	model.Protocol = types.Int32PointerValue(row.Protocol)
	model.SourceAddresses = stringsToList(row.LimitedAddresses)
	model.WanPortIds = stringsToList(row.InterfaceWanPortId)
	model.VirtualWanIds = stringsToList(row.VirtualWanId)
	model.WanIps = portIpsToMap(row.WanIps)
}
