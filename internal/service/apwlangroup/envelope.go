package apwlangroup

import (
	"encoding/json"
	"regexp"
)

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// apOverviewRead is a lenient, provider-local view of the AP overview payload
// returned by GET /aps/{apMac}. Only the fields the resource needs are decoded.
type apOverviewRead struct {
	Mac         string
	Name        string
	WlanGroupId string
}

// wlanGroupIDKeys are the candidate JSON keys the AP overview may carry the
// current WLAN group id under. On live firmware (6.2.10.18) the overview
// (`GET /aps/{apMac}`) keys it as "wlanId" — the same field the WLAN-group
// create returns — not the spaced "wlan group id" the SDK model ApOverviewInfo
// declares (a codegen artifact from the spec's field label). The decode tries
// each spelling; order is most-to-least likely, with the observed live key first.
var wlanGroupIDKeys = []string{
	"wlanId",
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
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return apOverviewRead{}, err
	}

	var ov apOverviewRead
	if v, ok := m["mac"].(string); ok {
		ov.Mac = v
	}
	if v, ok := m["name"].(string); ok {
		ov.Name = v
	}
	for _, k := range wlanGroupIDKeys {
		if v, ok := m[k].(string); ok && objectIDPattern.MatchString(v) {
			ov.WlanGroupId = v
			break
		}
	}
	return ov, nil
}
