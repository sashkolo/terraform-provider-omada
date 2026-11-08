package ipmacbinding

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// stringPtrOrNull maps a controller string pointer to a Terraform value,
// collapsing both nil and "" to null, so an unset optional stays stable.
func stringPtrOrNull(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// flattenBindingRead refreshes the model from a grid row. site_id is kept from
// the prior state. mac keeps the configured spelling when it names the same
// address as the controller's (case and separators aside), so a lower-case or
// colon-separated MAC in the configuration doesn't plan a change; a different
// address shows as drift. Every other field is read back from the controller.
func flattenBindingRead(m *bindingResourceModel, r *bindingReadRow) {
	if r == nil {
		return
	}
	m.BindingId = stringPtrOrNull(r.Id)
	if r.Mac != nil && (m.Mac.IsNull() || m.Mac.IsUnknown() || normalizeMAC(m.Mac.ValueString()) != normalizeMAC(*r.Mac)) {
		m.Mac = types.StringValue(*r.Mac)
	}
	m.Ip = stringPtrOrNull(r.Ip)
	m.InterfaceId = stringPtrOrNull(r.InterfaceId)
	if r.InterfaceType != nil {
		if name, ok := interfaceTypeName(*r.InterfaceType); ok {
			m.InterfaceType = types.StringValue(name)
		} else {
			// An unknown code: surface it as-is so the plan shows drift rather
			// than hiding it behind the default.
			m.InterfaceType = types.StringValue("unknown")
		}
	}
	m.Description = stringPtrOrNull(r.Description)
	// status and the export flag are always present on a live row; default
	// them if a row ever omits them so the computed values are known.
	m.Status = types.BoolValue(r.Status == nil || *r.Status)
	m.ExportToDhcpReservation = types.BoolValue(r.EnableExportToDhcpReservation != nil && *r.EnableExportToDhcpReservation)
}
