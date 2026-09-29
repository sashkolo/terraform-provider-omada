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
	// operation on the per-port modify body; this resource manages ordinary
	// switching ports.
	opSwitching = "switching"
)

// expandPort builds the SDK per-port setting sent on Create and Modify.
//
// This resource is a singleton that modifies an always-existing physical port,
// so the write must not clobber live settings the plan does not specify. Optional
// attributes that are Null/Unknown are therefore sent as nil pointers, which the
// controller treats as "leave unchanged" — the value is then read back and saved
// to state. Only profile_id (Required) is always sent; poe falls back to the
// "do not modify" mode when unset so PoE is never disturbed implicitly.
func expandPort(plan switchPortResourceModel) omada.OswPortSettingVO {
	profileId := plan.ProfileId.ValueString()
	operation := opSwitching

	var name *string
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		n := plan.Name.ValueString()
		name = &n
	}

	poe := poeModeDoNotModify
	if !plan.Poe.IsNull() && !plan.Poe.IsUnknown() {
		poe = plan.Poe.ValueInt32()
	}

	var override *bool
	if !plan.ProfileOverrideEnable.IsNull() && !plan.ProfileOverrideEnable.IsUnknown() {
		o := plan.ProfileOverrideEnable.ValueBool()
		override = &o
	}

	var disable *bool
	if !plan.Disabled.IsNull() && !plan.Disabled.IsUnknown() {
		d := plan.Disabled.ValueBool()
		disable = &d
	}

	return omada.OswPortSettingVO{
		Name:                  name,
		ProfileId:             &profileId,
		ProfileOverrideEnable: override,
		Poe:                   &poe,
		Disable:               disable,
		Operation:             &operation,
	}
}
