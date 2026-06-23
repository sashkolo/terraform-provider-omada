package ssid

import "github.com/Tohaker/omada-go-sdk/omada"

const (
	// errNotFound is the Omada Open API error code the controller returns for
	// a missing SSID on 5.15.x. Read/Delete use it to tolerate an SSID that is
	// already gone upstream.
	errNotFound int32 = -1001
)

// Defaults mirror the controller's own observed values on firmware 5.15.8.12
// (captured from a live WPA-Personal SSID during the #54 capability probe).
// They keep create payloads consistent with what the UI would produce while
// remaining overridable from configuration.
const (
	defaultBroadcast         bool  = true
	defaultGuestNetEnable    bool  = false
	defaultEnable11r         bool  = false
	defaultHidePwd           bool  = false
	defaultMloEnable         bool  = false
	defaultPmfMode           int32 = 2 // Capable
	defaultDeviceType        int32 = 3 // EAP + Gateway
	defaultVlanEnable        bool  = false
	defaultPskEncryption     int32 = 3
	defaultPskVersion        int32 = 4 // WPA2/WPA3 Personal compatibility
	defaultGikRekeyPskEnable bool  = false
)

func boolOrDefault(v interface {
	IsNull() bool
	IsUnknown() bool
	ValueBool() bool
}, def bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueBool()
}

func int32OrDefault(v interface {
	IsNull() bool
	IsUnknown() bool
	ValueInt32() int32
}, def int32) int32 {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueInt32()
}

// boolPtr returns a pointer to v; used for fields the controller requires to be
// present even when their SDK model declares them optional (e.g. greEnable on
// the v1 update-basic-config endpoint on 5.15.x).
func boolPtr(v bool) *bool {
	return &v
}

// expandPskSetting converts the optional psk_setting block into the SDK type.
// Returns nil when the block is absent so the SSID is created without a PSK
// (valid for security mode 0 / open). The cipher/version/rekey fields are
// always populated (they are required by the controller when a PSK is sent).
func expandPskSetting(s *pskSettingModel) *omada.SsidPskSettingOpenApiVO {
	if s == nil {
		return nil
	}

	vo := &omada.SsidPskSettingOpenApiVO{
		EncryptionPsk:     int32OrDefault(s.EncryptionPsk, defaultPskEncryption),
		VersionPsk:        int32OrDefault(s.VersionPsk, defaultPskVersion),
		GikRekeyPskEnable: boolOrDefault(s.GikRekeyPskEnable, defaultGikRekeyPskEnable),
	}

	// Only send the key when it is actually set; the controller rejects an
	// empty securityKey on create.
	if !s.Psk.IsNull() && !s.Psk.IsUnknown() && s.Psk.ValueString() != "" {
		v := s.Psk.ValueString()
		vo.SecurityKey = &v
	}

	return vo
}

// expandCreateSsid builds the SDK value sent on Create. All fields required by
// the v1 create endpoint are populated, defaulting any the operator left
// unset.
func expandCreateSsid(plan ssidResourceModel) omada.CreateSsidOpenApiVO {
	vlanEnable := boolOrDefault(plan.VlanEnable, defaultVlanEnable)

	vo := omada.CreateSsidOpenApiVO{
		Name:           plan.Name.ValueString(),
		Security:       plan.Security.ValueInt32(),
		Band:           plan.Band.ValueInt32(),
		Broadcast:      boolOrDefault(plan.Broadcast, defaultBroadcast),
		GuestNetEnable: boolOrDefault(plan.GuestNetEnable, defaultGuestNetEnable),
		Enable11r:      boolOrDefault(plan.Enable11r, defaultEnable11r),
		HidePwd:        boolOrDefault(plan.HidePwd, defaultHidePwd),
		MloEnable:      boolOrDefault(plan.MloEnable, defaultMloEnable),
		PmfMode:        int32OrDefault(plan.PmfMode, defaultPmfMode),
		DeviceType:     int32OrDefault(plan.DeviceType, defaultDeviceType),
		VlanEnable:     vlanEnable,
		// greEnable is optional in the SDK model but rejected if absent by the
		// 5.15.x update endpoint; send it explicitly (disabled) on create too
		// for consistency.
		GreEnable: boolPtr(false),
	}

	// vlanId is only valid (and required) when VLAN tagging is enabled.
	if vlanEnable && !plan.VlanId.IsNull() && !plan.VlanId.IsUnknown() {
		v := plan.VlanId.ValueInt32()
		vo.VlanId = &v
	}

	if plan.PskSetting != nil {
		vo.PskSetting = expandPskSetting(plan.PskSetting)
	}

	return vo
}

// expandUpdateSsid builds the SDK value sent on Update (the v1
// update-basic-config endpoint). The update surface does not carry deviceType;
// hidePwd is optional.
func expandUpdateSsid(plan ssidResourceModel) omada.UpdateSsidBasicConfigOpenApiVO {
	vlanEnable := boolOrDefault(plan.VlanEnable, defaultVlanEnable)

	hidePwd := boolOrDefault(plan.HidePwd, defaultHidePwd)
	vo := omada.UpdateSsidBasicConfigOpenApiVO{
		Name:           plan.Name.ValueString(),
		Security:       plan.Security.ValueInt32(),
		Band:           plan.Band.ValueInt32(),
		Broadcast:      boolOrDefault(plan.Broadcast, defaultBroadcast),
		GuestNetEnable: boolOrDefault(plan.GuestNetEnable, defaultGuestNetEnable),
		Enable11r:      boolOrDefault(plan.Enable11r, defaultEnable11r),
		HidePwd:        &hidePwd,
		MloEnable:      boolOrDefault(plan.MloEnable, defaultMloEnable),
		PmfMode:        int32OrDefault(plan.PmfMode, defaultPmfMode),
		VlanEnable:     vlanEnable,
		// greEnable is optional in the SDK model but rejected if absent by the
		// 5.15.x update-basic-config endpoint; send it explicitly (disabled).
		GreEnable: boolPtr(false),
	}

	if vlanEnable && !plan.VlanId.IsNull() && !plan.VlanId.IsUnknown() {
		v := plan.VlanId.ValueInt32()
		vo.VlanId = &v
	}

	if plan.PskSetting != nil {
		vo.PskSetting = expandPskSetting(plan.PskSetting)
	}

	return vo
}
