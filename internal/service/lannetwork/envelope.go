package lannetwork

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// createResult is the {id} payload of the v1 create endpoint.
type createResult struct {
	Id *string `json:"id"`
}

// dhcpReadVO mirrors the controller's dhcpSettingsVO shape on read.
type dhcpReadVO struct {
	// DHCP settings carried on update (carry.go); Options is modeled.
	DhcpNextServer *string          `json:"dhcpNextServer"`
	Option60       *string          `json:"option60"`
	Option66       *string          `json:"option66"`
	Option138      *string          `json:"option138"`
	Options        []dhcpOptionRead `json:"options"`

	Enable      *bool   `json:"enable"`
	Dhcpns      *string `json:"dhcpns"`
	Gateway     *string `json:"gateway"`
	IpaddrStart *string `json:"ipaddrStart"`
	IpaddrEnd   *string `json:"ipaddrEnd"`
	Leasetime   *int32  `json:"leasetime"`
	PriDns      *string `json:"priDns"`
	SndDns      *string `json:"sndDns"`
}

// dhcpOptionRead is one custom DHCP option on read.
type dhcpOptionRead struct {
	Code  *int32  `json:"code"`
	Type  *int32  `json:"type"`
	Value *string `json:"value"`
}

// lanNetworkReadRow is a lenient, provider-local view of one LAN-network list
// entry. Only the fields the resource cares about are named; everything else is
// ignored by the decoder.
type lanNetworkReadRow struct {
	Id              *string     `json:"id"`
	Name            string      `json:"name"`
	Vlan            *int32      `json:"vlan"`
	Purpose         int32       `json:"purpose"`
	GatewaySubnet   *string     `json:"gatewaySubnet"`
	Domain          *string     `json:"domain"`
	IgmpSnoopEnable bool        `json:"igmpSnoopEnable"`
	InterfaceIds    []string    `json:"interfaceIds"`
	DhcpSettingsVO  *dhcpReadVO `json:"dhcpSettingsVO"`
	// Primary marks the site's default network, whose listed name carries a
	// display suffix (see flattenName).
	Primary bool `json:"primary"`

	// Network settings carried or guarded on update (carry.go). Isolation is
	// modeled; LanNetworkIpv6Config is exposed read-only as ipv6_enabled.
	AllLan               *bool           `json:"allLan"`
	Application          *int32          `json:"application"`
	DhcpL2RelayEnable    *bool           `json:"dhcpL2RelayEnable"`
	Isolation            *bool           `json:"isolation"`
	MldSnoopEnable       *bool           `json:"mldSnoopEnable"`
	DhcpGuard            *enableFlag     `json:"dhcpGuard"`
	Dhcpv6Guard          *enableFlag     `json:"dhcpv6Guard"`
	LanNetworkIpv6Config *ipv6ConfigFlag `json:"lanNetworkIpv6Config"`
}

// enableFlag reads only the on/off switch of a nested setting object.
type enableFlag struct {
	Enable *bool `json:"enable"`
}

// ipv6ConfigFlag reads only whether a network's IPv6 is on (0 = off).
type ipv6ConfigFlag struct {
	Enable *int32 `json:"enable"`
}

// listResult is the paged list payload.
type listResult struct {
	Data []lanNetworkReadRow `json:"data"`
}
