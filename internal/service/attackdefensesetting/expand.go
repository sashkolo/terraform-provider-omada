package attackdefensesetting

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenSpecifiedOption converts the lenient read view into the Terraform block.
// Returns nil when the controller reports no sub-object.
func flattenSpecifiedOption(s *specifiedOptionReadVO) *specifiedOptionModel {
	if s == nil {
		return nil
	}
	return &specifiedOptionModel{
		NoOperationEnable:    types.BoolPointerValue(s.NoOperationEnable),
		RecordRouteEnable:    types.BoolPointerValue(s.RecordRouteEnable),
		SecurityOptionEnable: types.BoolPointerValue(s.SecurityEnable),
		StreamEnable:         types.BoolPointerValue(s.StreamEnable),
		TimestampEnable:      types.BoolPointerValue(s.TimestampEnable),
	}
}

// flattenAttackDefenseRead overwrites the resource model from a lenient read
// object. site_id is preserved from prior state on Read (it keys the singleton).
func flattenAttackDefenseRead(m *attackDefenseSettingResourceModel, r *attackDefenseReadVO) {
	if r == nil {
		return
	}
	m.IcmpConnEnable = types.BoolValue(r.IcmpConnEnable)
	m.IcmpConnLimit = types.Int32PointerValue(r.IcmpConnLimit)
	m.IcmpSrcEnable = types.BoolValue(r.IcmpSrcEnable)
	m.IcmpSrcLimit = types.Int32PointerValue(r.IcmpSrcLimit)
	m.IcmpTimestampRequestReject = types.BoolPointerValue(r.IcmpTimestampRequestReject)
	m.LargePingEnable = types.BoolValue(r.LargePingEnable)
	m.LargePingThreshold = types.Int32PointerValue(r.LargePingThreshold)
	m.PingDeathEnable = types.BoolValue(r.PingDeathEnable)
	m.PingWanEnable = types.BoolValue(r.PingWanEnable)
	m.SpecifiedOptionEnable = types.BoolValue(r.SpecifiedOptionEnable)
	m.SpecifiedOption = flattenSpecifiedOption(r.SpecifiedOption)
	m.TcpConnEnable = types.BoolValue(r.TcpConnEnable)
	m.TcpConnLimit = types.Int32PointerValue(r.TcpConnLimit)
	m.TcpFinNoAckEnable = types.BoolValue(r.TcpFinNoAckEnable)
	m.TcpScanEnable = types.BoolValue(r.TcpScanEnable)
	m.TcpScanReject = types.BoolPointerValue(r.TcpScanReject)
	m.TcpSrcEnable = types.BoolValue(r.TcpSrcEnable)
	m.TcpSrcLimit = types.Int32PointerValue(r.TcpSrcLimit)
	m.TcpSynFinEnable = types.BoolValue(r.TcpSynFinEnable)
	m.UdpConnEnable = types.BoolValue(r.UdpConnEnable)
	m.UdpConnLimit = types.Int32PointerValue(r.UdpConnLimit)
	m.UdpSrcEnable = types.BoolValue(r.UdpSrcEnable)
	m.UdpSrcLimit = types.Int32PointerValue(r.UdpSrcLimit)
	m.WinNukeAttackEnable = types.BoolValue(r.WinNukeAttackEnable)
}
