package switchport

import (
	"github.com/Tohaker/omada-go-sdk/omada"
)

const (
	// poeModeDoNotModify (2) leaves PoE unchanged; used as the write/read
	// default when the attribute is unset.
	poeModeDoNotModify int32 = 2

	// statusOff/statusOn are the controller's port admin-state values in the
	// read portList (0 off, 1 on).
	statusOff int32 = 0
	statusOn  int32 = 1

	// opSwitching is the default port operation. The controller expects an
	// operation on the per-port modify body; the homelab manages ordinary
	// switching ports.
	opSwitching = "switching"
)

// expandPort builds the SDK per-port setting sent on Create and Modify. Only the
// fields the resource models are set; the controller preserves the rest of the
// port configuration. profileOverrideEnable follows the plan so the port keeps
// (or clears) its custom fill mode as declared.
func expandPort(plan switchPortResourceModel) omada.OswPortSettingVO {
	profileId := plan.ProfileId.ValueString()
	name := plan.Name.ValueString()
	poe := poeModeDoNotModify
	if !plan.Poe.IsNull() && !plan.Poe.IsUnknown() {
		poe = plan.Poe.ValueInt32()
	}
	override := plan.ProfileOverrideEnable.ValueBool()
	disable := plan.Disabled.ValueBool()
	operation := opSwitching

	return omada.OswPortSettingVO{
		Name:                  &name,
		ProfileId:             &profileId,
		ProfileOverrideEnable: &override,
		Poe:                   &poe,
		Disable:               &disable,
		Operation:             &operation,
	}
}
