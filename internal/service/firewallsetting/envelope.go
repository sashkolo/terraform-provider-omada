package firewallsetting

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

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
