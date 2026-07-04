package switchportprofile

import (
	"context"

	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	// errProfileNotFound is the Omada Open API error code for a missing LAN
	// profile. Read/Delete use it to detect remote deletion.
	errProfileNotFound int32 = -33517

	// Controller defaults applied when the corresponding optional attribute is
	// left unset on create. They mirror the values a freshly-created LAN profile
	// carries on 5.15.x so an unspecified toggle round-trips without drift.
	defaultPoe               int32 = 2 // 2: "do not modify"
	defaultDot1x             int32 = 2 // 2: auto
	defaultBandWidthCtrlType int32 = 0 // 0: off
)

// expandNetworkIds converts a Terraform list of network IDs into the SDK slice.
// Always returns a non-nil slice so it serializes as [] when empty (an access
// port has no tagged networks; the controller distinguishes [] from absent).
func expandNetworkIds(ctx context.Context, list types.List) []string {
	out := []string{}
	if list.IsNull() || list.IsUnknown() {
		return out
	}
	var ids []string
	_ = list.ElementsAs(ctx, &ids, false)
	return append(out, ids...)
}

// int32OrDefault returns the attribute value, or def when it is null/unknown.
func int32OrDefault(v types.Int32, def int32) int32 {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueInt32()
}

// expandProfile builds the SDK LAN-profile config value sent on Create and
// Modify. Booleans default to false when unset (the controller's default for a
// new profile); numeric toggles fall back to the controller defaults above.
func expandProfile(ctx context.Context, plan switchPortProfileResourceModel) omada.LanProfileConfigOpenApiVO {
	tagged := expandNetworkIds(ctx, plan.TaggedNetworkIds)
	untagged := expandNetworkIds(ctx, plan.UntaggedNetworkIds)

	return omada.LanProfileConfigOpenApiVO{
		Name:                 plan.Name.ValueString(),
		NativeNetworkId:      plan.NativeNetworkId.ValueString(),
		TagNetworkIds:        tagged,
		UntagNetworkIds:      untagged,
		Poe:                  int32OrDefault(plan.Poe, defaultPoe),
		Dot1x:                int32OrDefault(plan.Dot1x, defaultDot1x),
		BandWidthCtrlType:    int32OrDefault(plan.BandWidthCtrlType, defaultBandWidthCtrlType),
		PortIsolationEnable:  plan.PortIsolationEnable.ValueBool(),
		LldpMedEnable:        plan.LldpMedEnable.ValueBool(),
		LoopbackDetectEnable: plan.LoopbackDetectEnable.ValueBool(),
		SpanningTreeEnable:   plan.SpanningTreeEnable.ValueBool(),
	}
}
