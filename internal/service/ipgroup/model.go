package ipgroup

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// groupTypeIP is the Omada group-profile type discriminator for an IP group.
// The Open API models IP groups, IP-Port groups, MAC groups, etc. as a single
// "group profile" object keyed by this type (0: IP Group; 1: IP-Port Group;
// 2: MAC Group; 3: IPv6 Group; 4: IPv6-Port Group; 5: Country; 7: Domain). The
// same value is the {groupType} path segment on read/modify/delete.
const groupTypeIP = "0"

// ipGroupClient is the SDK handle shared by the resource. It is populated from
// the provider Meta during Configure.
type ipGroupClient struct {
	client   *omada.APIClient
	omadacId string
}

// ipSubnetModel maps one entry of the ip_list nested block: a single host or a
// CIDR subnet the group matches. mask is the CIDR prefix length (1-32); a /32
// mask is a single host.
type ipSubnetModel struct {
	Ip          types.String `tfsdk:"ip"`
	Mask        types.Int32  `tfsdk:"mask"`
	Description types.String `tfsdk:"description"`
}

// ipGroupResourceModel maps the omada_ip_group resource schema. It models an
// Omada IP group profile: a reusable, named set of IP hosts/subnets that
// gateway ACL rules reference via source_type/destination_type = 1 (IP Group)
// instead of hard-coding whole-VLAN networks. The controller firmware targeted
// by this resource (Open API v1, e.g. 5.15.x/6.2.x) exposes the group surface
// at /openapi/v1/.../profiles/groups.
type ipGroupResourceModel struct {
	GroupId     types.String    `tfsdk:"group_id"`
	SiteId      types.String    `tfsdk:"site_id"`
	Name        types.String    `tfsdk:"name"`
	Description types.String    `tfsdk:"description"`
	IpList      []ipSubnetModel `tfsdk:"ip_list"`
}
