package wlangroup

import "github.com/Tohaker/omada-go-sdk/omada"

// expandCreateWlanGroup builds the SDK create value. A freshly created group
// never clones another group's SSIDs; it starts empty so the operator (or an
// omada_ssid resource) populates it explicitly.
func expandCreateWlanGroup(name string) omada.CreateWlanGroupOpenApiVO {
	return omada.CreateWlanGroupOpenApiVO{
		Name:  name,
		Clone: false,
	}
}

// expandUpdateWlanGroup builds the SDK update value. The v1 update surface only
// supports renaming a group, so this is name-only by design.
func expandUpdateWlanGroup(name string) omada.UpdateWlanGroupOpenApiVO {
	return omada.UpdateWlanGroupOpenApiVO{
		Name: name,
	}
}
