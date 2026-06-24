package firewallsetting

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// firewallSettingClient is the SDK handle shared by the resource. It is
// populated from the provider Meta during Configure.
type firewallSettingClient struct {
	client   *omada.APIClient
	omadacId string
}

// firewallSettingResourceModel maps the omada_firewall_setting resource schema.
// It models the site-global firewall settings: conntrack/protocol timeouts and
// a handful of hardening toggles. This is a singleton per site (the object
// always exists); the resource imports as `<site_id>` and Create/Update apply
// the whole object via Modify, Read fetches it via Get, and Delete is a no-op
// (the singleton cannot be removed; we never call Reset on destroy).
type firewallSettingResourceModel struct {
	SiteId           types.String `tfsdk:"site_id"`
	BroadcastPing    types.Bool   `tfsdk:"broadcast_ping"`
	Icmp             types.Int32  `tfsdk:"icmp"`
	Other            types.Int32  `tfsdk:"other"`
	ReceiveRedirects types.Bool   `tfsdk:"receive_redirects"`
	SendRedirects    types.Bool   `tfsdk:"send_redirects"`
	SynCookies       types.Bool   `tfsdk:"syn_cookies"`
	TcpClose         types.Int32  `tfsdk:"tcp_close"`
	TcpCloseWait     types.Int32  `tfsdk:"tcp_close_wait"`
	TcpEstablished   types.Int32  `tfsdk:"tcp_established"`
	TcpFinWait       types.Int32  `tfsdk:"tcp_fin_wait"`
	TcpLastAck       types.Int32  `tfsdk:"tcp_last_ack"`
	TcpSynReceive    types.Int32  `tfsdk:"tcp_syn_receive"`
	TcpSynSent       types.Int32  `tfsdk:"tcp_syn_sent"`
	TcpTimeWait      types.Int32  `tfsdk:"tcp_time_wait"`
	UdpOther         types.Int32  `tfsdk:"udp_other"`
	UdpStream        types.Int32  `tfsdk:"udp_stream"`
}
