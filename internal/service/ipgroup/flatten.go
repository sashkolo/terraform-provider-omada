package ipgroup

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// stringPtrOrNull maps a controller string pointer to a Terraform value,
// collapsing both nil and "" to null. The controller echoes an omitted optional
// as either absent or empty depending on firmware; normalizing here keeps an
// unset attribute stable across plans instead of flip-flopping "" vs null.
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

// flattenGroupRead overwrites the resource model from a lenient read row.
// site_id is preserved from the prior state (Read is keyed by group_id within a
// site); the remaining fields are refreshed from the controller.
func flattenGroupRead(m *ipGroupResourceModel, r *groupReadRow) {
	if r == nil {
		return
	}
	m.GroupId = types.StringPointerValue(r.GroupId)
	m.Name = types.StringValue(r.Name)
	// The controller stores the top-level group description but does NOT echo it
	// in the per-type group list read (confirmed live: name/ipList/portList come
	// back, description does not). Overwriting it with null here would trip
	// Terraform's "inconsistent result after apply" for this Optional attribute
	// and would drift on every refresh. So only adopt a description the read
	// actually returns; otherwise preserve the configured/prior value. (The
	// nested ip_list description *is* returned, so it flattens normally.) The
	// trade-off is that the description is not recoverable on bare import.
	if desc := stringPtrOrNull(r.Description); !desc.IsNull() {
		m.Description = desc
	}
	m.IpList = flattenIpList(r.IpList)
}
