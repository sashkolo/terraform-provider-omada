package ssid

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

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
	// AutoWanAccess is not modeled; Update sends the live value back so an edit
	// can't change it (homelab #515).
	AutoWanAccess *bool `json:"autoWanAccess"`
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

// wlanGroupRow is one entry of the site's WLAN-group list, used to tell an
// SSID list rejected because its group is gone from one rejected for another
// reason.
type wlanGroupRow struct {
	WlanId *string `json:"wlanId"`
}
