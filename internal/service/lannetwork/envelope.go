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
	Enable      *bool   `json:"enable"`
	Dhcpns      *string `json:"dhcpns"`
	Gateway     *string `json:"gateway"`
	IpaddrStart *string `json:"ipaddrStart"`
	IpaddrEnd   *string `json:"ipaddrEnd"`
	Leasetime   *int32  `json:"leasetime"`
	PriDns      *string `json:"priDns"`
	SndDns      *string `json:"sndDns"`
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
}

// listResult is the paged list payload.
type listResult struct {
	Data []lanNetworkReadRow `json:"data"`
}
