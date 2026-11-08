package ipmacbinding

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by internal/envelope.
// The SDK's grid model (IPMacBinding) has no id field, and its strict decoder
// rejects rows with fields it doesn't know, so the grid is decoded here.

// bindingGridResult mirrors the controller's paged grid response: the rows live
// under `data`, with `totalRows` describing the full set across pages.
type bindingGridResult struct {
	CurrentPage *int32           `json:"currentPage,omitempty"`
	CurrentSize *int32           `json:"currentSize,omitempty"`
	Data        []bindingReadRow `json:"data,omitempty"`
	TotalRows   *int64           `json:"totalRows,omitempty"`
}

// bindingReadRow is one IP-MAC binding grid entry. The deprecated
// exportToDhcpReservation and supportExport fields are ignored: they describe
// whether an export happened or is possible, not the configured setting.
type bindingReadRow struct {
	Id                            *string `json:"id,omitempty"`
	Mac                           *string `json:"mac,omitempty"`
	Ip                            *string `json:"ip,omitempty"`
	Description                   *string `json:"description,omitempty"`
	Status                        *bool   `json:"status,omitempty"`
	InterfaceType                 *int32  `json:"interfaceType,omitempty"`
	InterfaceId                   *string `json:"interfaceId,omitempty"`
	EnableExportToDhcpReservation *bool   `json:"enableExportToDhcpReservation,omitempty"`
}
