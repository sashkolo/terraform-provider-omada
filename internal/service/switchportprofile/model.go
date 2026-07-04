package switchportprofile

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// switchPortProfileClient is the SDK handle shared by the resource. It is
// populated from the provider Meta during Configure.
type switchPortProfileClient struct {
	client   *omada.APIClient
	omadacId string
}

// switchPortProfileResourceModel maps the omada_switch_port_profile resource
// schema. It models a reusable switch (OSW) LAN/port profile: the object a
// switch port is assigned to that defines the port's VLAN membership
// (native/untagged PVID plus tagged VLANs => access vs trunk) and a handful of
// L2 toggles.
//
// The controller firmware targeted by this resource (Open API v1, e.g.
// 5.15.8.12) exposes the profile surface at /openapi/v1/.../lan-profiles. The
// newer v2 surface (/openapi/v2/.../lan-profiles) returns 404 on that firmware,
// so this resource deliberately uses v1.
//
// A profile with a native network and no tagged networks is an access port on
// that VLAN; adding tagged networks makes it a trunk. Codifying the
// Outdoor-Untrusted profile (native VLAN 30, zero tagged) is how the homelab
// proves the outdoor cable stays access/untagged with no sensitive VLANs.
type switchPortProfileResourceModel struct {
	ProfileId            types.String `tfsdk:"profile_id"`
	SiteId               types.String `tfsdk:"site_id"`
	Name                 types.String `tfsdk:"name"`
	NativeNetworkId      types.String `tfsdk:"native_network_id"`
	TaggedNetworkIds     types.List   `tfsdk:"tagged_network_ids"`
	UntaggedNetworkIds   types.List   `tfsdk:"untagged_network_ids"`
	Poe                  types.Int32  `tfsdk:"poe"`
	Dot1x                types.Int32  `tfsdk:"dot1x"`
	BandWidthCtrlType    types.Int32  `tfsdk:"bandwidth_ctrl_type"`
	PortIsolationEnable  types.Bool   `tfsdk:"port_isolation_enable"`
	LldpMedEnable        types.Bool   `tfsdk:"lldp_med_enable"`
	LoopbackDetectEnable types.Bool   `tfsdk:"loopback_detect_enable"`
	SpanningTreeEnable   types.Bool   `tfsdk:"spanning_tree_enable"`
	Flag                 types.Int32  `tfsdk:"flag"`
	Type                 types.Int32  `tfsdk:"type"`
}
