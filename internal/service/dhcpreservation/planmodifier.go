package dhcpreservation

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// netNameFollowsNetID plans net_name from the prior state only while net_id
// stays the same. net_name is computed: the controller reports the name of the
// network the reservation belongs to. stringplanmodifier.UseStateForUnknown
// would carry the old network's name into the plan when net_id moves the
// reservation to another network, and the apply would then fail with
// "Provider produced inconsistent result after apply" once the controller
// reports the new name. When net_id changes, net_name is left unknown and is
// filled in by the read after the update.
type netNameFollowsNetID struct{}

func (netNameFollowsNetID) Description(_ context.Context) string {
	return "Uses the prior net_name while net_id is unchanged; otherwise net_name is known after apply."
}

func (m netNameFollowsNetID) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (netNameFollowsNetID) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Same guards as UseStateForUnknown: nothing to carry on create, and a
	// known or config-unknown plan value is left alone.
	if req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}

	var planNetID, stateNetID types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("net_id"), &planNetID)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("net_id"), &stateNetID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if planNetID.IsUnknown() || !planNetID.Equal(stateNetID) {
		return
	}

	resp.PlanValue = req.StateValue
}
