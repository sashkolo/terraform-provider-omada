package dhcpreservation

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors internal/service/ipgroup/envelope.go and the other homelab resources:
// the generated SDK decodes response bodies with DisallowUnknownFields and
// enforces every required property, which rejects fields the controller returns
// that the SDK model does not know about. These helpers re-read the SDK call's
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

// createResult is the {id} payload of the create endpoint (ResIdOpenApiVO): the
// reservation create returns the new id directly. The id is informational only
// (reservations are addressed by MAC), but it is surfaced as a computed
// attribute.
type createResult struct {
	Id *string `json:"id"`
}

// reservationGridResult mirrors the controller's paged grid response
// (DhcpReservationOpenApiGridVO): the reservation rows live under `data`, with
// `totalRows` describing the full set across pages. Only the fields the resource
// needs are named; everything else is ignored by the decoder.
type reservationGridResult struct {
	CurrentPage *int32               `json:"currentPage,omitempty"`
	CurrentSize *int32               `json:"currentSize,omitempty"`
	Data        []reservationReadRow `json:"data,omitempty"`
	TotalRows   *int64               `json:"totalRows,omitempty"`
}

// reservationReadRow is a lenient, provider-local view of one reservation grid
// entry. Only the fields the resource cares about are named; the many read-only
// diagnostics fields the controller returns (abnormal, model, serverMac,
// showingType, ...) are ignored by the decoder.
type reservationReadRow struct {
	Id          *string `json:"id,omitempty"`
	Mac         *string `json:"mac,omitempty"`
	Ip          *string `json:"ip,omitempty"`
	Description *string `json:"description,omitempty"`
	NetId       *string `json:"netId,omitempty"`
	NetName     *string `json:"netName,omitempty"`
	Status      *bool   `json:"status,omitempty"`
}
