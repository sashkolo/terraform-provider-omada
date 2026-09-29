package portforwarding

// createResult accepts both the generic id key and a future/type-specific key.
// Current controllers commonly omit result entirely for this endpoint, in
// which case Create resolves the rule by its unique name from the list.
type createResult struct {
	Id               *string `json:"id"`
	PortForwardingId *string `json:"portForwardingId"`
}

type portIpRead struct {
	Ip    *string `json:"ip,omitempty"`
	WanId *string `json:"wanId,omitempty"`
}

// portForwardingReadRow is a lenient view of a row returned by the paged list.
// DMZ rules are intentionally unsupported: omada_port_forwarding represents a
// least-privilege port/range mapping, not an all-ports exposure primitive.
type portForwardingReadRow struct {
	DMZ                bool         `json:"dMZ"`
	ExternalPort       *string      `json:"externalPort,omitempty"`
	ForwardIp          string       `json:"forwardIp"`
	ForwardPort        *string      `json:"forwardPort,omitempty"`
	From               int32        `json:"from"`
	Id                 *string      `json:"id,omitempty"`
	InterfaceWanPortId []string     `json:"interfaceWanPortId,omitempty"`
	LimitedAddresses   []string     `json:"limitedAddresses,omitempty"`
	Name               string       `json:"name"`
	Protocol           *int32       `json:"protocol,omitempty"`
	Status             bool         `json:"status"`
	VirtualWanId       []string     `json:"virtualWanId,omitempty"`
	WanIps             []portIpRead `json:"wanIps,omitempty"`
}

type listResult struct {
	CurrentPage *int32                  `json:"currentPage,omitempty"`
	CurrentSize *int32                  `json:"currentSize,omitempty"`
	Data        []portForwardingReadRow `json:"data,omitempty"`
	TotalRows   *int64                  `json:"totalRows,omitempty"`
}
