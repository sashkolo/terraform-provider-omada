package acl

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// aclClient is the SDK handle shared by the resource. It is populated from the
// provider Meta during Configure.
type aclClient struct {
	client   *omada.APIClient
	omadacId string
}

// directionModel maps the gateway ACL direction entity. At least one of the
// direction flags must be set; lan_to_lan conflicts with the other directions.
// vpn_in_ids / wan_in_ids apply when the corresponding direction is selected.
type directionModel struct {
	LanToLan types.Bool     `tfsdk:"lan_to_lan"`
	LanToWan types.Bool     `tfsdk:"lan_to_wan"`
	VpnInIds []types.String `tfsdk:"vpn_in_ids"`
	WanInIds []types.String `tfsdk:"wan_in_ids"`
}

// statesModel maps the optional gateway ACL conntrack-state match entity. It is
// only meaningful when state_mode is manual (1).
type statesModel struct {
	Established types.Bool `tfsdk:"established"`
	Invalid     types.Bool `tfsdk:"invalid"`
	Related     types.Bool `tfsdk:"related"`
	StateNew    types.Bool `tfsdk:"state_new"`
}

// aclResourceModel maps the omada_acl resource schema. It models a gateway (OSG)
// ACL rule: the core building block of inter-VLAN segmentation on the Omada
// gateway. The controller firmware targeted by this resource (Open API v1, e.g.
// 5.15.8.12) exposes the gateway ACL surface at /openapi/v1/.../acls/osg-acls.
//
// `index` (rule order) is read-only: the Omada Open API reorders ACLs via a
// site-global, per-device-type ModifyAclIndex call that carries the full ordered
// id map, so a single ACL resource cannot own its absolute index without
// fighting out-of-band changes. The resource therefore observes the
// controller-assigned index and never sends it.
type aclResourceModel struct {
	AclId           types.String   `tfsdk:"acl_id"`
	SiteId          types.String   `tfsdk:"site_id"`
	Index           types.Int32    `tfsdk:"index"`
	Description     types.String   `tfsdk:"description"`
	SourceType      types.Int32    `tfsdk:"source_type"`
	SourceIds       []types.String `tfsdk:"source_ids"`
	DestinationType types.Int32    `tfsdk:"destination_type"`
	// DestinationIds is Optional+Computed (the controller resolves ids for some
	// destination types). It must be types.List, not []types.String, so it can
	// represent the unknown value the framework holds during the create plan
	// before the controller populates it.
	DestinationIds types.List      `tfsdk:"destination_ids"`
	Policy         types.Int32     `tfsdk:"policy"`
	Protocols      []types.Int32   `tfsdk:"protocols"`
	StateMode      types.Int32     `tfsdk:"state_mode"`
	Status         types.Bool      `tfsdk:"status"`
	Syslog         types.Bool      `tfsdk:"syslog"`
	Direction      *directionModel `tfsdk:"direction"`
	States         *statesModel    `tfsdk:"states"`
	TimeRangeId    types.String    `tfsdk:"time_range_id"`
}
