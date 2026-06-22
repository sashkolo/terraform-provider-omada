package lannetwork

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// The generated SDK decodes response bodies with DisallowUnknownFields, which
// rejects fields the controller returns that the SDK model does not know about.
// Controller firmware 5.15.x returns e.g. "lanNetworkIpv6Config" (correct
// spelling) while the SDK model only knows the codegen typo "lanNeworkIpv6Config",
// so every list read failed with "json: unknown field". The deprecated create
// endpoint also returns only a bare {id} model, discarding the controller's
// errorCode/msg on validation failures (e.g. -33515 "LAN interfaces could not
// be none").
//
// To stay robust against SDK/controller drift while still using the SDK for the
// authenticated HTTP transport, these helpers re-read the SDK call's re-readable
// http.Response.Body and decode the envelope with the standard library (which
// ignores unknown fields). The provider thus surfaces real controller errors and
// tolerates field additions in either direction.

// omadaEnvelope is the standard {errorCode, msg, result} wrapper returned by
// every Open API endpoint. Result is captured raw and decoded per-call.
type omadaEnvelope struct {
	ErrorCode *int32          `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// hasError reports a controller-side error (non-zero errorCode). The "network
// not found" code is not an error for read/delete.
func (e omadaEnvelope) hasError() bool {
	return e.ErrorCode != nil && *e.ErrorCode != 0
}

// createResult is the {id} payload of the v1 create endpoint.
type createResult struct {
	Id *string `json:"id"`
}

// dhcpReadVO mirrors the controller's dhcpSettingsVO shape on read.
type dhcpReadVO struct {
	Enable      *bool   `json:"enable"`
	Dhcpns      *string `json:"dhcpns"`
	Gateway     *string `json:"gateway"`
	IpaddrStart *string `json:"ipaddrStart"`
	IpaddrEnd   *string `json:"ipaddrEnd"`
	Leasetime   *int32  `json:"leasetime"`
	PriDns      *string `json:"priDns"`
	SndDns      *string `json:"sndDns"`
}

// lanNetworkReadRow is a lenient, provider-local view of one LAN-network list
// entry. Only the fields the resource cares about are named; everything else is
// ignored by the decoder.
type lanNetworkReadRow struct {
	Id              *string     `json:"id"`
	Name            string      `json:"name"`
	Vlan            *int32      `json:"vlan"`
	Purpose         int32       `json:"purpose"`
	GatewaySubnet   *string     `json:"gatewaySubnet"`
	Domain          *string     `json:"domain"`
	IgmpSnoopEnable bool        `json:"igmpSnoopEnable"`
	InterfaceIds    []string    `json:"interfaceIds"`
	DhcpSettingsVO  *dhcpReadVO `json:"dhcpSettingsVO"`
}

// listResult is the paged list payload.
type listResult struct {
	Data []lanNetworkReadRow `json:"data"`
}
