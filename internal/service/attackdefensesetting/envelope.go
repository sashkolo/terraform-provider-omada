package attackdefensesetting

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// specifiedOptionReadVO mirrors the controller's specifiedOption shape (all
// optional pointers). NOTE: the controller returns "securityEnable" for the
// security-option toggle, while the SDK model (SpecifiedOptionOpenApiVO) uses
// the codegen name "securityOptionEnable"; the read VO uses the controller's
// actual key so the value round-trips into state (see flattenSpecifiedOption).
type specifiedOptionReadVO struct {
	NoOperationEnable *bool `json:"noOperationEnable,omitempty"`
	RecordRouteEnable *bool `json:"recordRouteEnable,omitempty"`
	SecurityEnable    *bool `json:"securityEnable,omitempty"`
	StreamEnable      *bool `json:"streamEnable,omitempty"`
	TimestampEnable   *bool `json:"timestampEnable,omitempty"`
}

// attackDefenseReadVO is a lenient, provider-local view of the attack-defense
// settings object returned by GET /attack-defense. Enable toggles are value
// bools; limits/rejects/threshold are optional pointers; specifiedOption is an
// optional nested object.
type attackDefenseReadVO struct {
	IcmpConnEnable             bool                   `json:"icmpConnEnable"`
	IcmpConnLimit              *int32                 `json:"icmpConnLimit,omitempty"`
	IcmpSrcEnable              bool                   `json:"icmpSrcEnable"`
	IcmpSrcLimit               *int32                 `json:"icmpSrcLimit,omitempty"`
	IcmpTimestampRequestReject *bool                  `json:"icmpTimestampRequestReject,omitempty"`
	LargePingEnable            bool                   `json:"largePingEnable"`
	LargePingThreshold         *int32                 `json:"largePingThreshold,omitempty"`
	PingDeathEnable            bool                   `json:"pingDeathEnable"`
	PingWanEnable              bool                   `json:"pingWanEnable"`
	SpecifiedOptionEnable      bool                   `json:"specifiedOptionEnable"`
	SpecifiedOption            *specifiedOptionReadVO `json:"specifiedOption,omitempty"`
	TcpConnEnable              bool                   `json:"tcpConnEnable"`
	TcpConnLimit               *int32                 `json:"tcpConnLimit,omitempty"`
	TcpFinNoAckEnable          bool                   `json:"tcpFinNoAckEnable"`
	TcpScanEnable              bool                   `json:"tcpScanEnable"`
	TcpScanReject              *bool                  `json:"tcpScanReject,omitempty"`
	TcpSrcEnable               bool                   `json:"tcpSrcEnable"`
	TcpSrcLimit                *int32                 `json:"tcpSrcLimit,omitempty"`
	TcpSynFinEnable            bool                   `json:"tcpSynFinEnable"`
	UdpConnEnable              bool                   `json:"udpConnEnable"`
	UdpConnLimit               *int32                 `json:"udpConnLimit,omitempty"`
	UdpSrcEnable               bool                   `json:"udpSrcEnable"`
	UdpSrcLimit                *int32                 `json:"udpSrcLimit,omitempty"`
	WinNukeAttackEnable        bool                   `json:"winNukeAttackEnable"`
}
