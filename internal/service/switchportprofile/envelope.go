package switchportprofile

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors internal/service/lannetwork/envelope.go and the other homelab
// resources: the generated SDK decodes response bodies with
// DisallowUnknownFields and enforces every required property, which rejects
// fields the controller returns that the SDK model does not know about. These
// helpers re-read the SDK call's re-readable http.Response.Body and decode with
// the standard library (which ignores unknown fields), so the provider tolerates
// SDK/controller drift while still using the SDK for the authenticated HTTP
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

// createResult is the {id} payload of the v1 create endpoint (ResponseIdVO).
type createResult struct {
	Id *string `json:"id"`
}

// profileReadRow is a lenient, provider-local view of one LAN-profile list
// entry. Only the fields the resource cares about are named; everything else is
// ignored by the decoder.
type profileReadRow struct {
	Id                   *string  `json:"id"`
	Name                 string   `json:"name"`
	NativeNetworkId      *string  `json:"nativeNetworkId"`
	TagNetworkIds        []string `json:"tagNetworkIds"`
	UntagNetworkIds      []string `json:"untagNetworkIds"`
	Poe                  int32    `json:"poe"`
	Dot1x                int32    `json:"dot1x"`
	BandWidthCtrlType    int32    `json:"bandWidthCtrlType"`
	PortIsolationEnable  bool     `json:"portIsolationEnable"`
	LldpMedEnable        bool     `json:"lldpMedEnable"`
	LoopbackDetectEnable bool     `json:"loopbackDetectEnable"`
	SpanningTreeEnable   bool     `json:"spanningTreeEnable"`
	Flag                 *int32   `json:"flag"`
	Type                 *int32   `json:"type"`
}

// listResult is the paged list payload { data: [ ... ] }.
type listResult struct {
	Data []profileReadRow `json:"data"`
}
