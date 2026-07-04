package ipportgroup

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// groupTypePort is the Omada group-profile type discriminator for an IP-Port
// group (0: IP Group; 1: IP-Port Group; 2: MAC; 3: IPv6; 4: IPv6-Port; ...).
// The same value is the {groupType} path segment on read/modify/delete.
const groupTypePort = "1"

// ipPortGroupClient is the SDK handle shared by the resource. It is populated
// from the provider Meta during Configure.
type ipPortGroupClient struct {
	client   *omada.APIClient
	omadacId string
}

// ipSubnetModel maps one entry of the ip_list nested block: a single host or a
// CIDR subnet the group scopes its ports to.
type ipSubnetModel struct {
	Ip          types.String `tfsdk:"ip"`
	Mask        types.Int32  `tfsdk:"mask"`
	Description types.String `tfsdk:"description"`
}

// portMaskModel maps one entry of the port_mask_list nested block, used when
// port_type is 1 (port/mask mode).
type portMaskModel struct {
	Port types.Int32  `tfsdk:"port"`
	Mask types.String `tfsdk:"mask"`
}

// ipPortGroupResourceModel maps the omada_ip_port_group resource schema. It
// models an Omada IP-Port group profile: a named set of IP hosts/subnets scoped
// to a set of TCP/UDP ports, which gateway ACL rules reference by group id
// (source_type/destination_type = 2, IP-Port Group). It is the right object for
// "allow host X to reach camera IPs only on the ONVIF/RTSP/HTTP service ports"
// policy. The controller exposes the group surface at
// /openapi/v1/.../profiles/groups.
type ipPortGroupResourceModel struct {
	GroupId      types.String    `tfsdk:"group_id"`
	SiteId       types.String    `tfsdk:"site_id"`
	Name         types.String    `tfsdk:"name"`
	Description  types.String    `tfsdk:"description"`
	IpList       []ipSubnetModel `tfsdk:"ip_list"`
	PortType     types.Int32     `tfsdk:"port_type"`
	PortList     []types.String  `tfsdk:"port_list"`
	PortMaskList []portMaskModel `tfsdk:"port_mask_list"`
}
