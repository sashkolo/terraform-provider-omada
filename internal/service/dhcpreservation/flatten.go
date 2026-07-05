package dhcpreservation

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// stringPtrOrNull maps a controller string pointer to a Terraform value,
// collapsing both nil and "" to null. The controller echoes an omitted optional
// as either absent or empty depending on firmware; normalizing here keeps an
// unset attribute stable across plans instead of flip-flopping "" vs null.
func stringPtrOrNull(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// flattenReservationRead overwrites the resource model from a lenient read row.
// site_id and mac are preserved from the prior state/config (Read is keyed by
// MAC within a site); the remaining fields are refreshed from the controller so
// a drifted ip, network, description or enable-state surfaces in the plan.
func flattenReservationRead(m *reservationResourceModel, r *reservationReadRow) {
	if r == nil {
		return
	}
	m.ReservationId = types.StringPointerValue(r.Id)
	m.Ip = stringPtrOrNull(r.Ip)
	m.NetId = stringPtrOrNull(r.NetId)
	m.NetName = stringPtrOrNull(r.NetName)
	m.Description = stringPtrOrNull(r.Description)
	// status is a bool the controller always returns for an existing reservation;
	// default it to true if a row ever omits it so the Computed value is known.
	if r.Status != nil {
		m.Status = types.BoolValue(*r.Status)
	} else {
		m.Status = types.BoolValue(true)
	}
}
