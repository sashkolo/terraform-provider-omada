package wlangroup

import "github.com/hashicorp/terraform-plugin-framework/types"

// flattenWlanGroupRead overwrites the resource model from a lenient read row.
// wlan_group_id and site_id are preserved from the prior state (Read is keyed
// by them); name and primary are refreshed from the controller.
func flattenWlanGroupRead(m *wlanGroupResourceModel, r *wlanGroupReadRow) {
	if r == nil {
		return
	}

	m.WlanId = types.StringPointerValue(r.WlanId)
	m.Name = types.StringValue(r.Name)
	m.Primary = types.BoolPointerValue(r.Primary)
}
