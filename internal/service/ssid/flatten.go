package ssid

import "github.com/hashicorp/terraform-plugin-framework/types"

// flattenPskRead converts the lenient PSK read view into the resource block.
// The securityKey is preserved from the prior model when the controller masks
// it (firmware policy may omit it), so Terraform never sees a false PSK drift
// and the sensitive value is never lost from state.
func flattenPskRead(m *pskSettingModel, r *pskReadVO) *pskSettingModel {
	if m == nil {
		m = &pskSettingModel{}
	}

	if r == nil {
		// No pskSetting upstream: keep whatever the operator configured. The
		// controller simply does not echo PSK state for non-PSK security modes.
		return m
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
	m.PskSetting = flattenPskRead(m.PskSetting, r.PskSetting)
}
