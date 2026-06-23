package acl

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors internal/service/lannetwork/envelope.go and
// internal/service/ssid/envelope.go: the generated SDK decodes response bodies
// with DisallowUnknownFields and enforces every required property, which rejects
// fields the controller returns that the SDK model does not know about (the same
// drift that broke lan_network reads). These helpers re-read the SDK call's
// re-readable http.Response.Body and decode with the standard library (which
// ignores unknown fields and missing-optionals), so the provider tolerates that
// drift while still using the SDK for the authenticated HTTP transport.

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

// createResult is the create payload. The controller returns the new id under a
// type-specific key ("aclId") rather than the generic "id" the SDK's
// OperationResponseWithoutResult implies; accept both so the id resolves
// straight from the create response without depending on a description lookup.
type createResult struct {
	Id    *string `json:"id"`
	AclId *string `json:"aclId"`
}

// directionReadVO mirrors the controller's direction shape on gateway ACL list
// reads. Fields are optional pointers; the decoder tolerates their absence.
type directionReadVO struct {
	LanToLan *bool    `json:"lanToLan,omitempty"`
	LanToWan *bool    `json:"lanToWan,omitempty"`
	VpnInIds []string `json:"vpnInIds,omitempty"`
	WanInIds []string `json:"wanInIds,omitempty"`
}

// statesReadVO mirrors the controller's states shape on gateway ACL list reads.
type statesReadVO struct {
	Established *bool `json:"established,omitempty"`
	Invalid     *bool `json:"invalid,omitempty"`
	Related     *bool `json:"related,omitempty"`
	StateNew    *bool `json:"stateNew,omitempty"`
}

// aclReadRow is a lenient, provider-local view of one gateway ACL list entry.
// Only the fields the resource cares about are named; everything else is
// ignored by the decoder. id and index are always returned by the controller for
// an existing ACL.
type aclReadRow struct {
	Id              string           `json:"id"`
	Index           int32            `json:"index"`
	Description     string           `json:"description"`
	SourceType      int32            `json:"sourceType"`
	SourceIds       []string         `json:"sourceIds"`
	DestinationType int32            `json:"destinationType"`
	DestinationIds  []string         `json:"destinationIds"`
	Policy          int32            `json:"policy"`
	Protocols       []int32          `json:"protocols"`
	StateMode       int32            `json:"stateMode"`
	Status          bool             `json:"status"`
	Syslog          bool             `json:"syslog"`
	Direction       *directionReadVO `json:"direction,omitempty"`
	States          *statesReadVO    `json:"states,omitempty"`
	TimeRangeId     *string          `json:"timeRangeId,omitempty"`
}

// listResult is the paged list payload { data: [ ... ] }.
type listResult struct {
	Data []aclReadRow `json:"data"`
}
