package switchport

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// flattenPortRead overwrites the resource model from a lenient read row.
// site_id, switch_mac and port are preserved from the prior state (Read is keyed
// by them); the remaining fields are refreshed from the controller.
func flattenPortRead(m *switchPortResourceModel, r *portReadRow) {
	if r == nil {
		return
	}

	m.Port = types.Int32Value(r.Port)
	m.Name = types.StringValue(r.Name)
	m.ProfileId = types.StringValue(r.ProfileId)
	m.ProfileName = types.StringValue(r.ProfileName)
	m.ProfileOverrideEnable = types.BoolValue(r.ProfileOverrideEnable)
	m.Poe = types.Int32Value(r.PoeMode)
	m.Disabled = types.BoolValue(r.Status == statusOff)
	m.LagPort = types.BoolValue(r.LagPort)
}
