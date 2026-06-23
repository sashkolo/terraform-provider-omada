package wlangroup

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors the approach in internal/service/lannetwork/envelope.go: the
// generated SDK decodes response bodies with DisallowUnknownFields, which
// rejects fields the controller returns that the SDK model does not know
// about. Controller firmware 5.15.x also returns shapes the SDK does not
// expect (e.g. the WLAN-group list is a bare array under "result", not the
// paged {"data":[...]} object the SDK assumes). These helpers re-read the
// SDK call's re-readable http.Response.Body and decode with the standard
// library (which ignores unknown fields), so the provider tolerates that
// drift while still using the SDK for the authenticated HTTP transport.

// omadaEnvelope is the standard {errorCode, msg, result} wrapper returned by
// every Open API endpoint. Result is captured raw and decoded per-call.
type omadaEnvelope struct {
	ErrorCode *int32          `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// hasError reports a controller-side error (non-zero errorCode). The
// "invalid request parameters" code (-1001) is surfaced by the controller when
// a WLAN group does not exist; it is not treated as an error for read/delete.
func (e omadaEnvelope) hasError() bool {
	return e.ErrorCode != nil && *e.ErrorCode != 0
}

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
