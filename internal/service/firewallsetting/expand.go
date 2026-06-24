package firewallsetting

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// expandFirewallSetting builds the SDK firewall settings value sent on Create
// and Modify. The whole object is sent on every write (it is a coarse blob).
func expandFirewallSetting(plan firewallSettingResourceModel) omada.FirewallSetting {
	return omada.FirewallSetting{
		BroadcastPing:    plan.BroadcastPing.ValueBool(),
		Icmp:             plan.Icmp.ValueInt32(),
		Other:            plan.Other.ValueInt32(),
		ReceiveRedirects: plan.ReceiveRedirects.ValueBool(),
		SendRedirects:    plan.SendRedirects.ValueBool(),
		SynCookies:       plan.SynCookies.ValueBool(),
		TcpClose:         plan.TcpClose.ValueInt32(),
		TcpCloseWait:     plan.TcpCloseWait.ValueInt32(),
		TcpEstablished:   plan.TcpEstablished.ValueInt32(),
		TcpFinWait:       plan.TcpFinWait.ValueInt32(),
		TcpLastAck:       plan.TcpLastAck.ValueInt32(),
		TcpSynReceive:    plan.TcpSynReceive.ValueInt32(),
		TcpSynSent:       plan.TcpSynSent.ValueInt32(),
		TcpTimeWait:      plan.TcpTimeWait.ValueInt32(),
		UdpOther:         plan.UdpOther.ValueInt32(),
		UdpStream:        plan.UdpStream.ValueInt32(),
	}
}

// flattenFirewallRead overwrites the resource model from a lenient read object.
// site_id is preserved from prior state on Read (it keys the singleton).
func flattenFirewallRead(m *firewallSettingResourceModel, r *firewallReadVO) {
	if r == nil {
		return
	}
	m.BroadcastPing = types.BoolValue(r.BroadcastPing)
	m.Icmp = types.Int32Value(r.Icmp)
	m.Other = types.Int32Value(r.Other)
	m.ReceiveRedirects = types.BoolValue(r.ReceiveRedirects)
	m.SendRedirects = types.BoolValue(r.SendRedirects)
	m.SynCookies = types.BoolValue(r.SynCookies)
	m.TcpClose = types.Int32Value(r.TcpClose)
	m.TcpCloseWait = types.Int32Value(r.TcpCloseWait)
	m.TcpEstablished = types.Int32Value(r.TcpEstablished)
	m.TcpFinWait = types.Int32Value(r.TcpFinWait)
	m.TcpLastAck = types.Int32Value(r.TcpLastAck)
	m.TcpSynReceive = types.Int32Value(r.TcpSynReceive)
	m.TcpSynSent = types.Int32Value(r.TcpSynSent)
	m.TcpTimeWait = types.Int32Value(r.TcpTimeWait)
	m.UdpOther = types.Int32Value(r.UdpOther)
	m.UdpStream = types.Int32Value(r.UdpStream)
}
