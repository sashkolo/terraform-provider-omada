package firewallsetting

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors internal/service/lannetwork/envelope.go: the generated SDK decodes
// response bodies with DisallowUnknownFields and enforces every required
// property, which rejects fields the controller returns that the SDK model does
// not know about (the same drift that broke every prior homelab read on 5.15.x).
// These helpers re-read the SDK call's re-readable http.Response.Body and decode
// with the standard library (which ignores unknown fields), so the provider
// tolerates that drift while still using the SDK for the authenticated HTTP
// transport.

// omadaEnvelope is the standard {errorCode, msg, result} wrapper returned by
// every Open API endpoint. Result is captured raw and decoded per-call.
type omadaEnvelope struct {
	ErrorCode *int32          `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// hasError reports a controller-side error (non-zero errorCode).
func (e omadaEnvelope) hasError() bool {
	return e.ErrorCode != nil && *e.ErrorCode != 0
}

// firewallReadVO is a lenient, provider-local view of the firewall settings
// object returned by GET /firewall. Every field is a value type on the wire.
type firewallReadVO struct {
	BroadcastPing    bool  `json:"broadcastPing"`
	Icmp             int32 `json:"icmp"`
	Other            int32 `json:"other"`
	ReceiveRedirects bool  `json:"receiveRedirects"`
	SendRedirects    bool  `json:"sendRedirects"`
	SynCookies       bool  `json:"synCookies"`
	TcpClose         int32 `json:"tcpClose"`
	TcpCloseWait     int32 `json:"tcpCloseWait"`
	TcpEstablished   int32 `json:"tcpEstablished"`
	TcpFinWait       int32 `json:"tcpFinWait"`
	TcpLastAck       int32 `json:"tcpLastAck"`
	TcpSynReceive    int32 `json:"tcpSynReceive"`
	TcpSynSent       int32 `json:"tcpSynSent"`
	TcpTimeWait      int32 `json:"tcpTimeWait"`
	UdpOther         int32 `json:"udpOther"`
	UdpStream        int32 `json:"udpStream"`
}
