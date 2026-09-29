package ipgroup

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// createResult is the {id} payload of the create endpoint (ResIdOpenApiVO): the
// group-profile create returns the new id directly, unlike the gateway ACL
// create which omits it.
type createResult struct {
	Id *string `json:"id"`
}

// ipSubnetReadVO mirrors the controller's ipList entry on group reads. Fields
// are lenient: description is an optional pointer the decoder tolerates absent.
type ipSubnetReadVO struct {
	Ip          string  `json:"ip"`
	Mask        int32   `json:"mask"`
	Description *string `json:"description,omitempty"`
}

// groupReadRow is a lenient, provider-local view of one group-profile list
// entry. Only the fields an IP group cares about are named; everything else
// (buildIn, count, port fields for other group types, ...) is ignored by the
// decoder.
type groupReadRow struct {
	GroupId     *string          `json:"groupId"`
	Name        string           `json:"name"`
	Description *string          `json:"description,omitempty"`
	Type        *int32           `json:"type,omitempty"`
	IpList      []ipSubnetReadVO `json:"ipList,omitempty"`
}
