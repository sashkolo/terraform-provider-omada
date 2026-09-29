package wlangroup

import "encoding/json"

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// createResult is the create payload. The controller returns the new id under
// a type-specific key ("wlanId") rather than the generic "id" the SDK's
// OperationResponse implies; accept both so the id resolves straight from the
// create response without depending on a name lookup.
type createResult struct {
	Id     *string `json:"id"`
	WlanId *string `json:"wlanId"`
}

// wlanGroupReadRow is a lenient, provider-local view of one WLAN-group list
// entry. Only the fields the resource cares about are named; everything else
// is ignored by the decoder. The controller's v1 list returns these as a bare
// array under "result".
type wlanGroupReadRow struct {
	WlanId  *string `json:"wlanId"`
	Name    string  `json:"name"`
	Primary *bool   `json:"primary"`
}

// wlanGroupListResult tolerates both the controller's bare-array list shape
// (5.15.x) and the SDK's expected paged {"data":[...]} shape. Decode fills
// Data from whichever is present.
type wlanGroupListResult struct {
	Data []wlanGroupReadRow `json:"data"`
}

// unwrapList accepts the raw "result" JSON and returns the list rows whether
// the controller emitted a bare array or a paged object.
func unwrapList(raw json.RawMessage) ([]wlanGroupReadRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Bare array: result: [ {...}, {...} ]
	var rows []wlanGroupReadRow
	if err := json.Unmarshal(raw, &rows); err == nil {
		return rows, nil
	}

	// Paged object: result: { data: [ ... ] }
	var paged wlanGroupListResult
	if err := json.Unmarshal(raw, &paged); err != nil {
		return nil, err
	}

	return paged.Data, nil
}
