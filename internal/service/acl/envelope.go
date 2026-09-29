package acl

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// createResult is the create payload. The controller returns the new id under a
// type-specific key ("aclId") rather than the generic "id" the SDK's
// OperationResponseWithoutResult implies; accept both so the id resolves
// straight from the create response without depending on a description lookup.
type createResult struct {
	Id    *string `json:"id"`
	AclId *string `json:"aclId"`
}

// directionReadVO mirrors the controller's direction shape on gateway ACL list
// reads. Fields are optional pointers; the decoder tolerates their absence.
type directionReadVO struct {
	LanToLan *bool    `json:"lanToLan,omitempty"`
	LanToWan *bool    `json:"lanToWan,omitempty"`
	VpnInIds []string `json:"vpnInIds,omitempty"`
	WanInIds []string `json:"wanInIds,omitempty"`
}

// statesReadVO mirrors the controller's states shape on gateway ACL list reads.
type statesReadVO struct {
	Established *bool `json:"established,omitempty"`
	Invalid     *bool `json:"invalid,omitempty"`
	Related     *bool `json:"related,omitempty"`
	StateNew    *bool `json:"stateNew,omitempty"`
}

// aclReadRow is a lenient, provider-local view of one gateway ACL list entry.
// Only the fields the resource cares about are named; everything else is
// ignored by the decoder. id and index are always returned by the controller for
// an existing ACL.
type aclReadRow struct {
	Id              string           `json:"id"`
	Index           int32            `json:"index"`
	Description     string           `json:"description"`
	SourceType      int32            `json:"sourceType"`
	SourceIds       []string         `json:"sourceIds"`
	DestinationType int32            `json:"destinationType"`
	DestinationIds  []string         `json:"destinationIds"`
	Policy          int32            `json:"policy"`
	Protocols       []int32          `json:"protocols"`
	StateMode       int32            `json:"stateMode"`
	Status          bool             `json:"status"`
	Syslog          bool             `json:"syslog"`
	Direction       *directionReadVO `json:"direction,omitempty"`
	States          *statesReadVO    `json:"states,omitempty"`
	TimeRangeId     *string          `json:"timeRangeId,omitempty"`
}

// listResult is the paged list payload { data: [ ... ] }.
type listResult struct {
	Data []aclReadRow `json:"data"`
}
