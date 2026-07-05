package dhcpreservation

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// reservationClient is the SDK handle shared by the resource. It is populated
// from the provider Meta during Configure.
type reservationClient struct {
	client   *omada.APIClient
	omadacId string
}

// reservationResourceModel maps the omada_dhcp_reservation resource schema. It
// models an Omada gateway DHCP fixed-IP reservation: a MAC-keyed binding that
// hands a specific client a stable address from a managed LAN network. The
// controller firmware targeted by this resource (Open API v1, e.g. 5.15.x /
// 6.2.x) exposes the reservation surface at
// /openapi/v1/.../setting/service/dhcp, keyed by MAC on modify/delete.
//
// The natural key is the MAC address (the {mac} path segment on modify/delete
// and the import id tail), not the controller-assigned reservation_id: the id
// is not known at import time and is not needed to address the reservation.
type reservationResourceModel struct {
	ReservationId types.String `tfsdk:"reservation_id"`
	SiteId        types.String `tfsdk:"site_id"`
	Mac           types.String `tfsdk:"mac"`
	Ip            types.String `tfsdk:"ip"`
	NetId         types.String `tfsdk:"net_id"`
	NetName       types.String `tfsdk:"net_name"`
	Description   types.String `tfsdk:"description"`
	Status        types.Bool   `tfsdk:"status"`
}
