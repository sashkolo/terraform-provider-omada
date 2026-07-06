package apwlangroup

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors internal/service/switchport/envelope.go and the other homelab
// resources: the generated SDK decodes response bodies with
// DisallowUnknownFields, which rejects fields the controller returns that the
// SDK model does not know about. These helpers re-read the SDK call's
// re-readable http.Response.Body and decode with the standard library (which
// ignores unknown fields), so the provider tolerates SDK/controller drift while
// still using the SDK for the authenticated HTTP transport.

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

// apOverviewRead is a lenient, provider-local view of the AP overview payload
// returned by GET /aps/{apMac}. Only the fields the resource needs are decoded.
//
// Note the WlanGroupId JSON tag: the controller (and the SDK model
// ApOverviewInfo) return the current WLAN group under the literal key
// "wlan group id" — with spaces — not a camelCase field.
type apOverviewRead struct {
	Mac         string `json:"mac"`
	Name        string `json:"name"`
	Model       string `json:"model"`
	WlanGroupId string `json:"wlan group id"`
}
