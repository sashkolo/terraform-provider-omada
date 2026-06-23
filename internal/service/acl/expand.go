package acl

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// listToStringSlice converts a Terraform types.List of strings into the SDK
// slice. Always returns a non-nil slice so it serializes as [] when empty.
// Tolerates null/unknown elements (e.g. during the create plan before the
// controller resolves destination ids).
func listToStringSlice(l types.List) []string {
	out := make([]string, 0, len(l.Elements()))
	for _, el := range l.Elements() {
		s, ok := el.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		out = append(out, s.ValueString())
	}
	return out
}

// expandStringList converts a Terraform list of strings into the SDK slice.
// Always returns a non-nil slice so it serializes as [] when empty.
func expandStringList(in []types.String) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v.IsNull() || v.IsUnknown() {
			continue
		}
		out = append(out, v.ValueString())
	}
	return out
}

// expandInt32List converts a Terraform list of int32 into the SDK slice. Always
// returns a non-nil slice so it serializes as [] when empty.
func expandInt32List(in []types.Int32) []int32 {
	out := make([]int32, 0, len(in))
	for _, v := range in {
		if v.IsNull() || v.IsUnknown() {
			continue
		}
		out = append(out, v.ValueInt32())
	}
	return out
}

// expandDirection converts the Terraform direction block into the SDK entity.
// The entity is returned by value (it is a required, non-pointer field on
// GatewayACLConfig).
func expandDirection(d *directionModel) omada.GatewayDirectionEntity {
	var e omada.GatewayDirectionEntity
	if d == nil {
		return e
	}
	e.LanToLan = d.LanToLan.ValueBoolPointer()
	e.LanToWan = d.LanToWan.ValueBoolPointer()
	e.VpnInIds = expandStringList(d.VpnInIds)
	e.WanInIds = expandStringList(d.WanInIds)
	return e
}

// expandStates converts the optional states block into the SDK entity. Returns
// nil when the block is absent so the ACL is created without conntrack-state
// matching.
func expandStates(s *statesModel) *omada.GatewayACLStatesEntity {
	if s == nil {
		return nil
	}
	return &omada.GatewayACLStatesEntity{
		Established: s.Established.ValueBoolPointer(),
		Invalid:     s.Invalid.ValueBoolPointer(),
		Related:     s.Related.ValueBoolPointer(),
		StateNew:    s.StateNew.ValueBoolPointer(),
	}
}

// expandGatewayACL builds the SDK gateway ACL value sent on Create and Modify.
// index is never set: rule order is controller-owned (see model.go).
func expandGatewayACL(plan aclResourceModel) omada.GatewayACLConfig {
	return omada.GatewayACLConfig{
		Description:     plan.Description.ValueString(),
		DestinationIds:  listToStringSlice(plan.DestinationIds),
		DestinationType: plan.DestinationType.ValueInt32(),
		Direction:       expandDirection(plan.Direction),
		Policy:          plan.Policy.ValueInt32(),
		Protocols:       expandInt32List(plan.Protocols),
		SourceIds:       expandStringList(plan.SourceIds),
		SourceType:      plan.SourceType.ValueInt32(),
		StateMode:       plan.StateMode.ValueInt32(),
		States:          expandStates(plan.States),
		Status:          plan.Status.ValueBool(),
		Syslog:          plan.Syslog.ValueBool(),
		TimeRangeId:     plan.TimeRangeId.ValueStringPointer(),
	}
}
