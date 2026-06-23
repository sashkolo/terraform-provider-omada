package ssid

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope.
//
// Mirrors internal/service/lannetwork/envelope.go: the generated SDK decodes
// response bodies with DisallowUnknownFields, which rejects fields the
// controller returns that the SDK model does not know about (e.g. 5.15.x
// returns extra sub-config blocks on SSID detail reads). These helpers
// re-read the SDK call's re-readable http.Response.Body and decode with the
// standard library (which ignores unknown fields), so the provider tolerates
// that drift while still using the SDK for the authenticated HTTP transport.

// omadaEnvelope is the standard {errorCode, msg, result} wrapper returned by
// every Open API endpoint. Result is captured raw and decoded per-call.
type omadaEnvelope struct {
	ErrorCode *int32          `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// hasError reports a controller-side error (non-zero errorCode). The
// "invalid request parameters" code (-1001) is returned when an SSID does not
// exist on 5.15.x; it is not treated as an error for read/delete.
func (e omadaEnvelope) hasError() bool {
	return e.ErrorCode != nil && *e.ErrorCode != 0
}

// createResult is the create payload. The controller returns the new id under a
// type-specific key ("ssidId") rather than the generic "id" the SDK's
// OperationResponse implies; accept both so the id resolves straight from the
// create response without depending on a name lookup.
type createResult struct {
	Id     *string `json:"id"`
	SsidId *string `json:"ssidId"`
}

// pskReadVO mirrors the controller's pskSetting shape on SSID detail reads.
// securityKey is optional: the controller may mask it depending on firmware
// policy; when absent the resource preserves the prior-state PSK.
type pskReadVO struct {
	SecurityKey       *string `json:"securityKey"`
	VersionPsk        *int32  `json:"versionPsk"`
	EncryptionPsk     *int32  `json:"encryptionPsk"`
	GikRekeyPskEnable *bool   `json:"gikRekeyPskEnable"`
}

// ssidDetailReadVO is a lenient, provider-local view of the SSID detail
// payload (GET /wlans/{wlanId}/ssids/{ssidId}). Only the fields the resource
// manages are named; everything else is ignored by the decoder.
type ssidDetailReadVO struct {
	SsidId         *string    `json:"ssidId"`
	Name           *string    `json:"name"`
	Band           *int32     `json:"band"`
	Broadcast      *bool      `json:"broadcast"`
	GuestNetEnable *bool      `json:"guestNetEnable"`
	Security       *int32     `json:"security"`
	VlanEnable     *bool      `json:"vlanEnable"`
	VlanId         *int32     `json:"vlanId"`
	PmfMode        *int32     `json:"pmfMode"`
	Enable11r      *bool      `json:"enable11r"`
	MloEnable      *bool      `json:"mloEnable"`
	HidePwd        *bool      `json:"hidePwd"`
	DeviceType     *int32     `json:"deviceType"`
	PskSetting     *pskReadVO `json:"pskSetting"`
}

// ssidListRow is one entry of the paged SSID list (GET .../ssids), used to
// resolve the ssidId by name after create when the create response omits it.
type ssidListRow struct {
	SsidId *string `json:"ssidId"`
	Name   *string `json:"name"`
}

// ssidListResult is the paged list payload { data: [ ... ] }.
type ssidListResult struct {
	Data []ssidListRow `json:"data"`
}
