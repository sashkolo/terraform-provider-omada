package switchport

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// portReadRow is a lenient, provider-local view of one switch port entry from
// the switch-overview portList. These are exactly the per-port fields the
// controller exposes on read on 5.15.x.
type portReadRow struct {
	Port                  int32  `json:"port"`
	Name                  string `json:"name"`
	ProfileId             string `json:"profileId"`
	ProfileName           string `json:"profileName"`
	ProfileOverrideEnable bool   `json:"profileOverrideEnable"`
	PoeMode               int32  `json:"poeMode"`
	Status                int32  `json:"status"`
	LagPort               bool   `json:"lagPort"`
}

// switchOverviewResult is the switch-overview payload; only portList is decoded.
type switchOverviewResult struct {
	PortList []portReadRow `json:"portList"`
}
