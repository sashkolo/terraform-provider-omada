package attackdefensesetting

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors the other homelab services: the generated SDK decodes response bodies
// with DisallowUnknownFields and enforces every required property, which rejects
// fields the controller returns that the SDK model does not know about (the drift
// that broke every prior read on 5.15.x). These helpers re-read the SDK call's
// re-readable http.Response.Body and decode with the standard library (which
// ignores unknown fields), so the provider tolerates that drift while still
// using the SDK for the authenticated HTTP transport.

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

// specifiedOptionReadVO mirrors the controller's specifiedOption shape (all
// optional pointers).
type specifiedOptionReadVO struct {
	NoOperationEnable    *bool `json:"noOperationEnable,omitempty"`
	RecordRouteEnable    *bool `json:"recordRouteEnable,omitempty"`
	SecurityOptionEnable *bool `json:"securityOptionEnable,omitempty"`
	StreamEnable         *bool `json:"streamEnable,omitempty"`
	TimestampEnable      *bool `json:"timestampEnable,omitempty"`
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
