package apwlangroup

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenOverviewRead overwrites the resource model from a lenient AP overview
// read. site_id and ap_mac are the immutable key (RequiresReplace) and are
// preserved from prior state; wlan_group_id and the read-only ap_name are
// refreshed from the controller.
func flattenOverviewRead(m *apWlanGroupResourceModel, r *apOverviewRead) {
	if r == nil {
		return
	}

	m.WlanGroupId = types.StringValue(r.WlanGroupId)
	m.ApName = types.StringValue(r.Name)
}
