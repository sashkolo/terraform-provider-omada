package switchportprofile

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenNetworkIds converts the controller's network ID list into a Terraform
// list value. A nil/empty controller list becomes an empty (non-null) list so
// an access port's empty tagged list is stable across plans.
func flattenNetworkIds(ids []string) types.List {
	elems := make([]attr.Value, 0, len(ids))
	for _, id := range ids {
		elems = append(elems, types.StringValue(id))
	}
	// types.ListValueMust never errs for a homogeneous []attr.Value of strings.
	return types.ListValueMust(types.StringType, elems)
}

// flattenProfileRead overwrites the resource model from a lenient read row.
// site_id is preserved from the prior state (Read is keyed by profile_id within
// a site); the remaining fields are refreshed from the controller.
func flattenProfileRead(m *switchPortProfileResourceModel, r *profileReadRow) {
	if r == nil {
		return
	}

	m.ProfileId = types.StringPointerValue(r.Id)
	m.Name = types.StringValue(r.Name)
	m.NativeNetworkId = types.StringPointerValue(r.NativeNetworkId)
	m.TaggedNetworkIds = flattenNetworkIds(r.TagNetworkIds)
	m.UntaggedNetworkIds = flattenNetworkIds(r.UntagNetworkIds)
	m.Poe = types.Int32Value(r.Poe)
	m.Dot1x = types.Int32Value(r.Dot1x)
	m.BandWidthCtrlType = types.Int32Value(r.BandWidthCtrlType)
	m.PortIsolationEnable = types.BoolValue(r.PortIsolationEnable)
	m.LldpMedEnable = types.BoolValue(r.LldpMedEnable)
	m.LoopbackDetectEnable = types.BoolValue(r.LoopbackDetectEnable)
	m.SpanningTreeEnable = types.BoolValue(r.SpanningTreeEnable)
	m.Flag = types.Int32PointerValue(r.Flag)
	m.Type = types.Int32PointerValue(r.Type)
}
