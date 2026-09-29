package switchportprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"terraform-provider-omada/internal/envelope"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// devicePageSize is the page size for the site device list.
const devicePageSize int32 = 100

// deviceTypeSwitch is the device list's type for a switch.
const deviceTypeSwitch = "switch"

// deviceRow is the part of a site device list entry the in-use check needs.
type deviceRow struct {
	Mac  string `json:"mac"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type deviceList struct {
	TotalRows int         `json:"totalRows"`
	Data      []deviceRow `json:"data"`
}

// switchPortRow is the part of a switch overview port the check needs.
type switchPortRow struct {
	Port      int32  `json:"port"`
	ProfileId string `json:"profileId"`
}

type switchOverview struct {
	PortList []switchPortRow `json:"portList"`
}

// refuseDeleteInUse walks every switch on the site and refuses the delete when
// a port still uses the profile. omada_switch_port's Delete leaves the port as
// it is, so destroying a port binding together with its profile would
// otherwise delete a profile that a live port still carries, and what the
// switch does with that port is undocumented. A failed lookup refuses too: it
// is no evidence that the profile is unused.
//
// Moving the ports to another profile in the same apply is refused too:
// Terraform and OpenTofu destroy this profile before they update the ports
// that no longer reference it. With create_before_destroy on the profile the
// order flips and the ports are updated first.
func (r *switchPortProfileResource) refuseDeleteInUse(ctx context.Context, diags *diag.Diagnostics, state *switchPortProfileResourceModel) bool {
	const action = "deleting switch port profile"
	site := state.SiteId.ValueString()
	id := state.ProfileId.ValueString()

	switches, ok := r.listSwitches(ctx, diags, site, action)
	if !ok {
		return true
	}

	var users []string
	for _, sw := range switches {
		ports, ok := r.switchPorts(ctx, diags, site, sw.Mac, action)
		if !ok {
			return true
		}
		for _, p := range ports {
			if p.ProfileId == id {
				users = append(users, fmt.Sprintf("%s port %d", switchLabel(sw), p.Port))
			}
		}
	}
	if len(users) == 0 {
		return false
	}

	diags.AddError("Error "+action, fmt.Sprintf(
		"Profile %q is still used by %d switch port(s): %s. Move those ports to another profile first "+
			"(omada_switch_port.profile_id, or the controller UI), then delete the profile in a later apply. "+
			"Destroying an omada_switch_port does not change the port, so it does not free the profile. "+
			"Moving the ports in the same apply is not enough on its own, because the profile is deleted "+
			"before the ports are updated, unless the profile was applied with "+
			"lifecycle { create_before_destroy = true }.",
		state.Name.ValueString(), len(users), strings.Join(users, ", ")))
	return true
}

func switchLabel(sw deviceRow) string {
	if sw.Name == "" || sw.Name == sw.Mac {
		return "switch " + sw.Mac
	}
	return fmt.Sprintf("switch %q (%s)", sw.Name, sw.Mac)
}

// listSwitches returns every switch in the site's device list.
func (r *switchPortProfileResource) listSwitches(ctx context.Context, diags *diag.Diagnostics, site, action string) ([]deviceRow, bool) {
	var switches []deviceRow
	seen := 0
	for page := int32(1); ; page++ {
		_, httpResp, callErr := r.client.DeviceAPI.GetDeviceList(ctx, r.omadacId, site).
			Page(page).PageSize(devicePageSize).Execute()
		env, ok := envelope.Decode(httpResp, callErr, diags, action)
		if !ok {
			return nil, false
		}
		if env.HasError() {
			envelope.AddAPIError(diags, action+" (listing the site's switches)", env.ErrorCode, env.Msg)
			return nil, false
		}
		var dl deviceList
		if err := json.Unmarshal(env.Result, &dl); err != nil {
			diags.AddError("Error "+action, "Could not decode the site device list: "+err.Error())
			return nil, false
		}
		for _, d := range dl.Data {
			if d.Type == deviceTypeSwitch {
				switches = append(switches, d)
			}
		}
		seen += len(dl.Data)
		if len(dl.Data) == 0 || seen >= dl.TotalRows {
			return switches, true
		}
	}
}

// switchPorts returns the ports of one switch from its overview.
func (r *switchPortProfileResource) switchPorts(ctx context.Context, diags *diag.Diagnostics, site, mac, action string) ([]switchPortRow, bool) {
	_, httpResp, callErr := r.client.SwitchAPI.GetSwitchInfo(ctx, r.omadacId, site, mac).Execute()
	env, ok := envelope.Decode(httpResp, callErr, diags, action)
	if !ok {
		return nil, false
	}
	if env.HasError() {
		envelope.AddAPIError(diags, fmt.Sprintf("%s (reading the ports of switch %s)", action, mac), env.ErrorCode, env.Msg)
		return nil, false
	}
	var so switchOverview
	if err := json.Unmarshal(env.Result, &so); err != nil {
		diags.AddError("Error "+action, fmt.Sprintf("Could not decode the overview of switch %s: %s", mac, err.Error()))
		return nil, false
	}
	return so.PortList, true
}
