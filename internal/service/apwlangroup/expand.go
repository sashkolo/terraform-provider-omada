package apwlangroup

import (
	"github.com/Tohaker/omada-go-sdk/omada"
)

// expandSwitch builds the SDK body sent on the "switch AP's wlan group" PATCH.
// The endpoint takes a single required field, the target WLAN group id.
func expandSwitch(plan apWlanGroupResourceModel) omada.ApUpdateWlanGroupOpenApiVO {
	return omada.ApUpdateWlanGroupOpenApiVO{
		WlanGroupId: plan.WlanGroupId.ValueString(),
	}
}
