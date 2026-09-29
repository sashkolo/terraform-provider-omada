package dhcpreservation

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

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
