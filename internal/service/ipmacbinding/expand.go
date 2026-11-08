package ipmacbinding

import (
	"strings"

	"github.com/Tohaker/omada-go-sdk/omada"
)

// Interface types as the Open API encodes them (interfaceType).
const (
	interfaceTypeWAN = "wan"
	interfaceTypeLAN = "lan"
)

var interfaceTypeCodes = map[string]int32{
	interfaceTypeWAN: 0,
	interfaceTypeLAN: 1,
}

func interfaceTypeName(code int32) (string, bool) {
	for name, c := range interfaceTypeCodes {
		if c == code {
			return name, true
		}
	}
	return "", false
}

// expandBinding builds the body sent on Create and Modify. Every field is
// sent, so a modify never leaves a stale value behind.
//
// mac is sent in the controller's canonical form (canonicalMAC).
//
// description: the controller accepts 1 to 64 characters. An unset description
// is omitted on create. On modify, clearing a description that the prior state
// held sends "" (clearDescription), because an omitted field leaves the old
// text in place.
//
// The deprecated exportToDhcpReservation and supportExport fields are never
// sent.
func expandBinding(plan bindingResourceModel, clearDescription bool) omada.IPMacBinding {
	body := omada.IPMacBinding{
		Mac:                           canonicalMAC(plan.Mac.ValueString()),
		Ip:                            plan.Ip.ValueString(),
		InterfaceType:                 interfaceTypeCodes[plan.InterfaceType.ValueString()],
		InterfaceId:                   plan.InterfaceId.ValueString(),
		Status:                        plan.Status.ValueBool(),
		EnableExportToDhcpReservation: plan.ExportToDhcpReservation.ValueBoolPointer(),
	}
	if d := plan.Description.ValueString(); !plan.Description.IsNull() && d != "" {
		body.Description = &d
	} else if clearDescription {
		empty := ""
		body.Description = &empty
	}
	return body
}

// canonicalMAC writes a MAC the way the controller stores it,
// AA-BB-CC-11-22-33, whatever case and separators the configuration uses. The
// configured spelling stays in state (see flattenBindingRead).
func canonicalMAC(s string) string {
	n := strings.ToUpper(normalizeMAC(s))
	if len(n) != 12 {
		return s
	}
	parts := make([]string, 0, 6)
	for i := 0; i < 12; i += 2 {
		parts = append(parts, n[i:i+2])
	}
	return strings.Join(parts, "-")
}
