package ssid

import "github.com/hashicorp/terraform-plugin-framework/types"

// flattenPskRead converts the lenient PSK read view into the resource block.
// The securityKey is preserved from the prior model when the controller masks
// it (firmware policy may omit it), so Terraform never sees a false PSK drift
// and the sensitive value is never lost from state.
//
// This is only called for WPA-Personal SSIDs (security == 3); callers must
// clear psk_setting for other modes (see flattenSsidRead). When the controller
// returns no pskSetting at all, the prior model is returned unchanged so a
// masked key is not lost. This nil-check must come before the m == nil init.
func flattenPskRead(m *pskSettingModel, r *pskReadVO) *pskSettingModel {
	if r == nil {
		// No pskSetting upstream for a PSK SSID: the controller is masking the
		// key. Keep the prior model so the sensitive value is not lost. (For
		// non-PSK modes the caller clears psk_setting directly.)
		return m
	}

	if m == nil {
		m = &pskSettingModel{}
	}

	// Preserve the prior PSK when the controller masks the key; otherwise
	// refresh from the read so state tracks out-of-band PSK changes.
	if r.SecurityKey != nil && *r.SecurityKey != "" {
		m.Psk = types.StringValue(*r.SecurityKey)
	}

	m.EncryptionPsk = types.Int32PointerValue(r.EncryptionPsk)
	m.VersionPsk = types.Int32PointerValue(r.VersionPsk)
	m.GikRekeyPskEnable = types.BoolPointerValue(r.GikRekeyPskEnable)

	return m
}

// securityPersonal is the WPA-Personal (PSK) security mode: the only mode that
// carries a psk_setting. All other modes (open, enterprise, PPSK) must not keep
// a psk_setting block in state.
const securityPersonal int32 = 3

// flattenSsidRead overwrites the resource model from a lenient detail read.
// ssid_id, site_id and wlan_group_id are preserved from the prior state (Read
// is keyed by them); the remaining fields are refreshed from the controller.
func flattenSsidRead(m *ssidResourceModel, r *ssidDetailReadVO) {
	if r == nil {
		return
	}

	m.SsidId = types.StringPointerValue(r.SsidId)
	m.Name = types.StringPointerValue(r.Name)
	m.Band = types.Int32PointerValue(r.Band)
	m.Broadcast = types.BoolPointerValue(r.Broadcast)
	m.GuestNetEnable = types.BoolPointerValue(r.GuestNetEnable)
	m.Security = types.Int32PointerValue(r.Security)
	m.VlanEnable = types.BoolPointerValue(r.VlanEnable)
	m.VlanId = types.Int32PointerValue(r.VlanId)
	m.PmfMode = types.Int32PointerValue(r.PmfMode)
	m.Enable11r = types.BoolPointerValue(r.Enable11r)
	m.MloEnable = types.BoolPointerValue(r.MloEnable)
	m.HidePwd = types.BoolPointerValue(r.HidePwd)
	m.DeviceType = types.Int32PointerValue(r.DeviceType)

	// psk_setting only applies to WPA-Personal. For any other mode the
	// controller returns no pskSetting; clear it from state so a stale block
	// does not linger after an out-of-band PSK -> non-PSK mode change. For PSK
	// mode, flatten normally (preserving the prior key if the controller masks
	// it on read).
	if r.Security != nil && *r.Security != securityPersonal {
		m.PskSetting = nil
	} else {
		m.PskSetting = flattenPskRead(m.PskSetting, r.PskSetting)
	}
}
