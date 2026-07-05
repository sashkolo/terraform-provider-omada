package dhcpreservation

import (
	"github.com/Tohaker/omada-go-sdk/omada"
)

// nonEmptyStringPointer returns p only when it points at a non-empty string,
// otherwise nil. It keeps an omitted optional out of the request body so the
// controller's own "" vs absent handling does not manufacture drift.
func nonEmptyStringPointer(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	return p
}

// expandReservation builds the SDK reservation config value sent on Create and
// Modify. mac, netId and status are required by the Open API body; ip is
// optional in the schema but always sent for a fixed reservation. A blank
// description is dropped rather than sent as "".
//
// ConfirmConflict is intentionally left unset (nil): the controller then
// rejects a create that collides with an existing IP-MAC binding or lease
// instead of force-overwriting it, which is the safe default for managing live
// reservations. An operator who needs to adopt a conflicting entry resolves the
// conflict in the controller first.
func expandReservation(plan reservationResourceModel) omada.CreateDhcpReservationOpenApiVO {
	return omada.CreateDhcpReservationOpenApiVO{
		Mac:         plan.Mac.ValueString(),
		NetId:       plan.NetId.ValueString(),
		Ip:          nonEmptyStringPointer(plan.Ip.ValueStringPointer()),
		Status:      plan.Status.ValueBool(),
		Description: nonEmptyStringPointer(plan.Description.ValueStringPointer()),
	}
}
