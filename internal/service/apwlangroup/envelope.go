package apwlangroup

import (
	"encoding/json"
	"regexp"
)

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
type apOverviewRead struct {
	Mac         string
	Name        string
	WlanGroupId string
}

// wlanGroupIDKeys are the candidate JSON keys the AP overview may carry the
// current WLAN group id under. The SDK model ApOverviewInfo declares the spaced
// key "wlan group id" (a codegen artifact from the spec's field label), but live
// firmware has been observed to key it differently (or to camelCase it), so the
// decode tries each spelling. Order is most-to-least likely.
var wlanGroupIDKeys = []string{
	"wlanGroupId",
	"wlan group id",
	"wlanGroupID",
	"wlan_group_id",
	"wlangroupid",
}

// objectIDPattern matches an Omada object id (a 24-char hex string). The WLAN
// group id must be an id, not a name, so a key that carries the group *name*
// (e.g. "Default") is rejected rather than mistaken for the id.
var objectIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)

// decodeApOverview leniently extracts the AP name and current WLAN group id from
// the overview result. The group id is resolved by scanning wlanGroupIDKeys and
// accepting the first id-shaped value; if none is present, WlanGroupId is empty
// and the caller treats the group as unreadable from this endpoint.
func decodeApOverview(raw json.RawMessage) (apOverviewRead, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return apOverviewRead{}, err
	}

	var ov apOverviewRead
	if v, ok := m["mac"]; ok {
		_ = json.Unmarshal(v, &ov.Mac)
	}
	if v, ok := m["name"]; ok {
		_ = json.Unmarshal(v, &ov.Name)
	}
	for _, k := range wlanGroupIDKeys {
		v, ok := m[k]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(v, &s) == nil && objectIDPattern.MatchString(s) {
			ov.WlanGroupId = s
			break
		}
	}
	return ov, nil
}
