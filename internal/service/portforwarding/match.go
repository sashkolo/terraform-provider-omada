package portforwarding

import (
	"maps"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// rowMatchesModel reports whether an eventually-consistent list row reflects
// the desired model. Create and Update use this before flattening so a stale row
// with the correct id cannot be committed to Terraform state.
func rowMatchesModel(row *portForwardingReadRow, model *portForwardingResourceModel) bool {
	if row == nil || row.DMZ ||
		row.Name != model.Name.ValueString() ||
		row.Status != model.Status.ValueBool() ||
		row.ExternalPort == nil || *row.ExternalPort != model.ExternalPort.ValueString() ||
		row.ForwardIp != model.ForwardIp.ValueString() ||
		row.ForwardPort == nil || *row.ForwardPort != model.ForwardPort.ValueString() ||
		row.Protocol == nil || *row.Protocol != model.Protocol.ValueInt32() {
		return false
	}

	if !listMatchesModel(row.LimitedAddresses, model.SourceAddresses) ||
		!listMatchesModel(row.InterfaceWanPortId, model.WanPortIds) ||
		!listMatchesModel(row.VirtualWanId, model.VirtualWanIds) ||
		!wanIpsMatchModel(row.WanIps, model.WanIps) {
		return false
	}

	if !model.SourceAddresses.IsNull() && !model.SourceAddresses.IsUnknown() {
		expectedFrom := int32(0)
		if len(listToStrings(model.SourceAddresses)) > 0 {
			expectedFrom = 1
		}
		if row.From != expectedFrom {
			return false
		}
	}

	return true
}

// Optional+Computed collections can still be unknown on create when omitted;
// in that case the controller chooses the computed value and there is nothing
// concrete to compare yet. Known lists retain ordering because the schema uses
// ListAttribute and Terraform treats order as part of the value.
func listMatchesModel(row []string, model types.List) bool {
	if model.IsNull() || model.IsUnknown() {
		return true
	}
	return slices.Equal(row, listToStrings(model))
}

func wanIpsMatchModel(row []portIpRead, model types.Map) bool {
	if model.IsNull() || model.IsUnknown() {
		return true
	}

	actual := make(map[string]string, len(row))
	for _, value := range row {
		if value.WanId == nil || *value.WanId == "" || value.Ip == nil || *value.Ip == "" {
			continue
		}
		actual[*value.WanId] = *value.Ip
	}

	expected := make(map[string]string, len(model.Elements()))
	for wanID, value := range model.Elements() {
		ip, ok := value.(types.String)
		if !ok || ip.IsNull() || ip.IsUnknown() {
			continue
		}
		expected[wanID] = ip.ValueString()
	}

	return maps.Equal(actual, expected)
}
