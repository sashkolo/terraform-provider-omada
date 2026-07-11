package portforwarding

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// portForwardingClient is the SDK handle shared by the resource. It is
// populated from the provider Meta during Configure.
type portForwardingClient struct {
	client   *omada.APIClient
	omadacId string
}

// portForwardingResourceModel maps omada_port_forwarding. Omada addresses a
// rule by its controller-assigned id and exposes reads through a paged site-wide
// list, so import uses <site_id>/<port_forwarding_id>.
type portForwardingResourceModel struct {
	PortForwardingId types.String `tfsdk:"port_forwarding_id"`
	SiteId           types.String `tfsdk:"site_id"`
	Name             types.String `tfsdk:"name"`
	Status           types.Bool   `tfsdk:"status"`
	ExternalPort     types.String `tfsdk:"external_port"`
	ForwardIp        types.String `tfsdk:"forward_ip"`
	ForwardPort      types.String `tfsdk:"forward_port"`
	Protocol         types.Int32  `tfsdk:"protocol"`
	SourceAddresses  types.List   `tfsdk:"source_addresses"`
	WanPortIds       types.List   `tfsdk:"wan_port_ids"`
	VirtualWanIds    types.List   `tfsdk:"virtual_wan_ids"`
	WanIps           types.Map    `tfsdk:"wan_ips"`
}
