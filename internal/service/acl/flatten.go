package acl

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenStringList converts the controller's string list into the Terraform
// list. Returns nil for an empty input so an absent value stays null in state
// (returning [] here would trip Terraform's "inconsistent result after apply"
// check for an Optional attribute that was null in the plan).
func flattenStringList(in []string) []types.String {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.String, 0, len(in))
	for _, v := range in {
		out = append(out, types.StringValue(v))
	}
	return out
}

// stringSliceToList converts the controller's string list into a Terraform
// types.List (the destination_ids model field is Optional+Computed, so it uses
// types.List to tolerate the unknown value held during the create plan).
func stringSliceToList(in []string) types.List {
	els := make([]attr.Value, 0, len(in))
	for _, v := range in {
		els = append(els, types.StringValue(v))
	}
	list, _ := types.ListValue(types.StringType, els)
	return list
}

// flattenInt32List converts the controller's int32 list into the Terraform list.
// Returns nil for an empty input (see flattenStringList).
func flattenInt32List(in []int32) []types.Int32 {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.Int32, 0, len(in))
	for _, v := range in {
		out = append(out, types.Int32Value(v))
	}
	return out
}

// flattenDirection converts the lenient local direction read view into the
// Terraform block. Returns nil when the controller reports no direction (the
// schema marks direction required, so this only happens for a malformed row).
func flattenDirection(d *directionReadVO) *directionModel {
	if d == nil {
		return nil
	}
	return &directionModel{
		LanToLan: types.BoolPointerValue(d.LanToLan),
		LanToWan: types.BoolPointerValue(d.LanToWan),
		VpnInIds: flattenStringList(d.VpnInIds),
		WanInIds: flattenStringList(d.WanInIds),
	}
}

// flattenStates converts the lenient local states read view into the Terraform
// block. Returns nil when the controller reports no states entity.
func flattenStates(s *statesReadVO) *statesModel {
	if s == nil {
		return nil
	}
	return &statesModel{
		Established: types.BoolPointerValue(s.Established),
		Invalid:     types.BoolPointerValue(s.Invalid),
		Related:     types.BoolPointerValue(s.Related),
		StateNew:    types.BoolPointerValue(s.StateNew),
	}
}

// flattenAclRead overwrites the resource model from a lenient read row. acl_id
// and site_id are preserved from the prior state on Read (Read is keyed by
// them); the remaining fields, including the read-only index, are refreshed
// from the controller.
func flattenAclRead(m *aclResourceModel, r *aclReadRow) {
	if r == nil {
		return
	}
	m.AclId = types.StringValue(r.Id)
	m.Index = types.Int32Value(r.Index)
	m.Description = types.StringValue(r.Description)
	m.SourceType = types.Int32Value(r.SourceType)
	m.SourceIds = flattenStringList(r.SourceIds)
	m.DestinationType = types.Int32Value(r.DestinationType)
	m.DestinationIds = stringSliceToList(r.DestinationIds)
	m.Policy = types.Int32Value(r.Policy)
	m.Protocols = flattenInt32List(r.Protocols)
	m.StateMode = types.Int32Value(r.StateMode)
	m.Status = types.BoolValue(r.Status)
	m.Syslog = types.BoolValue(r.Syslog)
	m.Direction = flattenDirection(r.Direction)
	m.States = flattenStates(r.States)
	m.TimeRangeId = types.StringPointerValue(r.TimeRangeId)
}
