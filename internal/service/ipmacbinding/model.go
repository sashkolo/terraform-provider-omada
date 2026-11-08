package ipmacbinding

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// bindingClient is the SDK handle shared by the resource. It is populated from
// the provider Meta during Configure.
type bindingClient struct {
	client   *omada.APIClient
	omadacId string
}

// bindingResourceModel maps the omada_gateway_ip_mac_binding schema: one entry
// of the gateway's IP-MAC binding table (Device Config -> Gateway -> IP-MAC
// Binding in the controller UI), which pins a client MAC to an IP on a gateway
// interface. The gateway keeps a static ARP entry for each bound pair, so the
// pair survives the client going quiet (for example a sleeping host that is
// woken by a directed wake-on-LAN packet).
//
// The controller addresses an entry by its binding id on modify/delete. The id
// isn't known before import, so import takes the MAC and Read resolves the id.
type bindingResourceModel struct {
	BindingId               types.String `tfsdk:"binding_id"`
	SiteId                  types.String `tfsdk:"site_id"`
	Mac                     types.String `tfsdk:"mac"`
	Ip                      types.String `tfsdk:"ip"`
	InterfaceType           types.String `tfsdk:"interface_type"`
	InterfaceId             types.String `tfsdk:"interface_id"`
	Description             types.String `tfsdk:"description"`
	Status                  types.Bool   `tfsdk:"status"`
	ExportToDhcpReservation types.Bool   `tfsdk:"export_to_dhcp_reservation"`
}
