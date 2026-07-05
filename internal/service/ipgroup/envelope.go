package ipgroup

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

// createResult is the {id} payload of the create endpoint (ResIdOpenApiVO): the
// group-profile create returns the new id directly, unlike the gateway ACL
// create which omits it.
type createResult struct {
	Id *string `json:"id"`
}

// ipSubnetReadVO mirrors the controller's ipList entry on group reads. Fields
// are lenient: description is an optional pointer the decoder tolerates absent.
type ipSubnetReadVO struct {
	Ip          string  `json:"ip"`
	Mask        int32   `json:"mask"`
	Description *string `json:"description,omitempty"`
}

// groupReadRow is a lenient, provider-local view of one group-profile list
// entry. Only the fields an IP group cares about are named; everything else
// (buildIn, count, port fields for other group types, ...) is ignored by the
// decoder.
type groupReadRow struct {
	GroupId     *string          `json:"groupId"`
	Name        string           `json:"name"`
	Description *string          `json:"description,omitempty"`
	Type        *int32           `json:"type,omitempty"`
	IpList      []ipSubnetReadVO `json:"ipList,omitempty"`
}
