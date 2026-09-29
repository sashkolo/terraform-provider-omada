package switchport

import (
	"context"
	"encoding/json"
	"fmt"

	"terraform-provider-omada/internal/envelope"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// profileTypeAll is the controller's "All" profile type: a trunk that carries
// every network. Access profiles are type 2.
const profileTypeAll int32 = 0

// profileRow is the part of a LAN profile the guard needs.
type profileRow struct {
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	Type          *int32   `json:"type"`
	TagNetworkIds []string `json:"tagNetworkIds"`
}

type profileList struct {
	Data []profileRow `json:"data"`
}

// guardPortWrite refuses a port write that could cut the network off before
// anything is sent. The Open API reports no uplink or
// management port, so the guard judges by what the port carries now:
//
//   - the port must exist on the switch (a typo in `port` used to be applied to
//     whichever port it named, or fail only after the write);
//   - a member of a link-aggregation group is refused unless allow_lag_member
//     is set: the write's "switching" operation takes it out of its LAG;
//   - moving a port off a trunk profile (the "All" type, or any profile with
//     tagged networks) is refused unless allow_trunk_reassign is set. Uplinks
//     and AP trunks sit on such profiles, and moving one to an access profile
//     cuts off whatever is behind it.
//
// It returns false, with an error diagnostic, when the write must not happen.
func (r *switchPortResource) guardPortWrite(ctx context.Context, diags *diag.Diagnostics, plan switchPortResourceModel, action string) bool {
	live, ok := r.fetchPortRow(ctx, diags, plan)
	if !ok {
		return false
	}
	if live == nil {
		diags.AddError("Error "+action, fmt.Sprintf("Switch %s has no port %d.", plan.SwitchMac.ValueString(), plan.Port.ValueInt32()))
		return false
	}

	if live.LagPort && !plan.AllowLagMember.ValueBool() {
		diags.AddError("Error "+action, fmt.Sprintf(
			"Port %d on switch %s is a link-aggregation member. Writing it would take it out of its LAG; set "+
				"allow_lag_member = true if that is intended.", live.Port, plan.SwitchMac.ValueString()))
		return false
	}

	if live.ProfileId == plan.ProfileId.ValueString() || plan.AllowTrunkReassign.ValueBool() {
		return true
	}
	current, ok := r.fetchProfile(ctx, diags, plan, live.ProfileId, action)
	if !ok {
		return false
	}
	if current != nil && isTrunk(current) {
		diags.AddError("Error "+action, fmt.Sprintf(
			"Port %d on switch %s is on the trunk profile %q, which carries %d tagged network(s). Moving it to "+
				"another profile would cut off whatever is behind it (an uplink, an AP, another switch). Set "+
				"allow_trunk_reassign = true if that is intended.",
			live.Port, plan.SwitchMac.ValueString(), current.Name, len(current.TagNetworkIds)))
		return false
	}
	return true
}

func isTrunk(p *profileRow) bool {
	return (p.Type != nil && *p.Type == profileTypeAll) || len(p.TagNetworkIds) > 0
}

// fetchPortRow returns the switch's current row for the plan's port, or nil
// when the switch has no such port.
func (r *switchPortResource) fetchPortRow(ctx context.Context, diags *diag.Diagnostics, plan switchPortResourceModel) (*portReadRow, bool) {
	_, httpResp, callErr := r.client.SwitchAPI.GetSwitchInfo(ctx, r.omadacId, plan.SiteId.ValueString(), plan.SwitchMac.ValueString()).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, "reading switch port")
	if !ok {
		return nil, false
	}
	if env.HasError() {
		envelope.AddAPIError(diags, "reading switch port", env.ErrorCode, env.Msg)
		return nil, false
	}
	var sr switchOverviewResult
	if err := json.Unmarshal(env.Result, &sr); err != nil {
		diags.AddError("Error reading switch port", "Could not decode switch overview: "+err.Error())
		return nil, false
	}
	for i := range sr.PortList {
		if sr.PortList[i].Port == plan.Port.ValueInt32() {
			return &sr.PortList[i], true
		}
	}
	return nil, true
}

// fetchProfile returns the site's LAN profile with the given id, or nil when
// it isn't listed.
func (r *switchPortResource) fetchProfile(ctx context.Context, diags *diag.Diagnostics, plan switchPortResourceModel, id, action string) (*profileRow, bool) {
	_, httpResp, callErr := r.client.WiredNetworkAPI.GetLanProfileList(ctx, r.omadacId, plan.SiteId.ValueString()).
		Page(1).PageSize(1000).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, action)
	if !ok {
		return nil, false
	}
	if env.HasError() {
		envelope.AddAPIError(diags, action, env.ErrorCode, env.Msg)
		return nil, false
	}
	var lr profileList
	if err := json.Unmarshal(env.Result, &lr); err != nil {
		diags.AddError("Error "+action, "Could not decode LAN-profile list: "+err.Error())
		return nil, false
	}
	for i := range lr.Data {
		if lr.Data[i].Id == id {
			return &lr.Data[i], true
		}
	}
	return nil, true
}
