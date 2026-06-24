package attackdefensesetting

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// attackDefenseSettingClient is the SDK handle shared by the resource. It is
// populated from the provider Meta during Configure.
type attackDefenseSettingClient struct {
	client   *omada.APIClient
	omadacId string
}

// specifiedOptionModel maps the optional IP-option attack-defense sub-object
// (each toggle is an optional pointer on the wire).
type specifiedOptionModel struct {
	NoOperationEnable    types.Bool `tfsdk:"no_operation_enable"`
	RecordRouteEnable    types.Bool `tfsdk:"record_route_enable"`
	SecurityOptionEnable types.Bool `tfsdk:"security_option_enable"`
	StreamEnable         types.Bool `tfsdk:"stream_enable"`
	TimestampEnable      types.Bool `tfsdk:"timestamp_enable"`
}

// attackDefenseSettingResourceModel maps the omada_attack_defense_setting
// resource schema. It models the site-global attack-defense (DoS/flood/scan
// protection) settings: a set of enable toggles (required) plus per-category
// rate limits and a few reject/timestamp flags (optional pointers). This is a
// singleton per site (the object always exists); Create/Update apply the whole
// object via Modify, Read fetches it via Get, Delete is a no-op (the singleton
// cannot be removed; we never call Reset on destroy), and the import id is
// `<site_id>`.
type attackDefenseSettingResourceModel struct {
	SiteId                     types.String          `tfsdk:"site_id"`
	IcmpConnEnable             types.Bool            `tfsdk:"icmp_conn_enable"`
	IcmpConnLimit              types.Int32           `tfsdk:"icmp_conn_limit"`
	IcmpSrcEnable              types.Bool            `tfsdk:"icmp_src_enable"`
	IcmpSrcLimit               types.Int32           `tfsdk:"icmp_src_limit"`
	IcmpTimestampRequestReject types.Bool            `tfsdk:"icmp_timestamp_request_reject"`
	LargePingEnable            types.Bool            `tfsdk:"large_ping_enable"`
	LargePingThreshold         types.Int32           `tfsdk:"large_ping_threshold"`
	PingDeathEnable            types.Bool            `tfsdk:"ping_death_enable"`
	PingWanEnable              types.Bool            `tfsdk:"ping_wan_enable"`
	SpecifiedOptionEnable      types.Bool            `tfsdk:"specified_option_enable"`
	SpecifiedOption            *specifiedOptionModel `tfsdk:"specified_option"`
	TcpConnEnable              types.Bool            `tfsdk:"tcp_conn_enable"`
	TcpConnLimit               types.Int32           `tfsdk:"tcp_conn_limit"`
	TcpFinNoAckEnable          types.Bool            `tfsdk:"tcp_fin_no_ack_enable"`
	TcpScanEnable              types.Bool            `tfsdk:"tcp_scan_enable"`
	TcpScanReject              types.Bool            `tfsdk:"tcp_scan_reject"`
	TcpSrcEnable               types.Bool            `tfsdk:"tcp_src_enable"`
	TcpSrcLimit                types.Int32           `tfsdk:"tcp_src_limit"`
	TcpSynFinEnable            types.Bool            `tfsdk:"tcp_syn_fin_enable"`
	UdpConnEnable              types.Bool            `tfsdk:"udp_conn_enable"`
	UdpConnLimit               types.Int32           `tfsdk:"udp_conn_limit"`
	UdpSrcEnable               types.Bool            `tfsdk:"udp_src_enable"`
	UdpSrcLimit                types.Int32           `tfsdk:"udp_src_limit"`
	WinNukeAttackEnable        types.Bool            `tfsdk:"win_nuke_attack_enable"`
}
