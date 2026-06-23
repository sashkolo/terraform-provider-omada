package ssid

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ssidClient is the SDK handle shared by the resource. It is populated from
// the provider Meta during Configure.
type ssidClient struct {
	client   *omada.APIClient
	omadacId string
}

// pskSettingModel maps the WPA-Personal key material attached to an SSID.
// The psk attribute itself is Sensitive at the schema level; these knobs
// (cipher, version, rekey) default to the controller's own observed defaults
// but remain configurable so the resource is fully drift-detectable.
type pskSettingModel struct {
	Psk               types.String `tfsdk:"psk"`
	EncryptionPsk     types.Int32  `tfsdk:"psk_encryption"`
	VersionPsk        types.Int32  `tfsdk:"psk_version"`
	GikRekeyPskEnable types.Bool   `tfsdk:"gik_rekey_psk_enable"`
}

// ssidResourceModel maps the omada_ssid resource schema. An SSID always lives
// inside a WLAN group; wlan_group_id is therefore required and forces
// replacement (SSIDs cannot be moved between groups).
//
// The controller firmware targeted by this resource (Open API v1, e.g.
// 5.15.8.12) exposes the SSID surface at
// /openapi/v1/.../wireless-network/wlans/{wlanId}/ssids.
type ssidResourceModel struct {
	SsidId         types.String     `tfsdk:"ssid_id"`
	SiteId         types.String     `tfsdk:"site_id"`
	WlanGroupId    types.String     `tfsdk:"wlan_group_id"`
	Name           types.String     `tfsdk:"name"`
	Security       types.Int32      `tfsdk:"security"`
	Band           types.Int32      `tfsdk:"band"`
	Broadcast      types.Bool       `tfsdk:"broadcast"`
	GuestNetEnable types.Bool       `tfsdk:"guest_net_enable"`
	Enable11r      types.Bool       `tfsdk:"enable_11r"`
	HidePwd        types.Bool       `tfsdk:"hide_pwd"`
	MloEnable      types.Bool       `tfsdk:"mlo_enable"`
	PmfMode        types.Int32      `tfsdk:"pmf_mode"`
	DeviceType     types.Int32      `tfsdk:"device_type"`
	VlanEnable     types.Bool       `tfsdk:"vlan_enable"`
	VlanId         types.Int32      `tfsdk:"vlan_id"`
	PskSetting     *pskSettingModel `tfsdk:"psk_setting"`
}
