package switchport

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors internal/service/lannetwork/envelope.go and the other homelab
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

// portReadRow is a lenient, provider-local view of one switch port entry from
// the switch-overview portList. These are exactly the per-port fields the
// controller exposes on read on 5.15.x.
type portReadRow struct {
	Port                  int32  `json:"port"`
	Name                  string `json:"name"`
	ProfileId             string `json:"profileId"`
	ProfileName           string `json:"profileName"`
	ProfileOverrideEnable bool   `json:"profileOverrideEnable"`
	PoeMode               int32  `json:"poeMode"`
	Status                int32  `json:"status"`
	LagPort               bool   `json:"lagPort"`
}

// switchOverviewResult is the switch-overview payload; only portList is decoded.
type switchOverviewResult struct {
	PortList []portReadRow `json:"portList"`
}
