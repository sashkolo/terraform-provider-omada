package ipgroup

import (
	"github.com/Tohaker/omada-go-sdk/omada"
)

// groupBodyTypeIP is the IP group discriminator in the create/modify body. It
// mirrors the groupTypeIP path segment: the body carries the type as an integer
// while the URL carries it as a string.
const groupBodyTypeIP int32 = 0

// nonEmptyStringPointer returns p only when it points at a non-empty string,
// otherwise nil. It keeps an omitted optional out of the request body so the
// controller's own "" vs absent handling does not manufacture drift.
func nonEmptyStringPointer(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	return p
}

// expandIpList converts the ip_list nested block into the SDK slice. Always
// returns a non-nil slice so it serializes as [] when empty (the controller
// distinguishes [] from absent). A blank per-entry description is dropped rather
// than sent as "".
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

// expandGroup builds the SDK group-profile config value sent on Create and
// Modify for an IP group (type 0).
func expandGroup(plan ipGroupResourceModel) omada.CreateGroupOpenApiVO {
	return omada.CreateGroupOpenApiVO{
		Name:        plan.Name.ValueString(),
		Description: nonEmptyStringPointer(plan.Description.ValueStringPointer()),
		IpList:      expandIpList(plan.IpList),
		Type:        groupBodyTypeIP,
	}
}
