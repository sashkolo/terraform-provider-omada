package switchportprofile

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// createResult is the {id} payload of the v1 create endpoint (ResponseIdVO).
type createResult struct {
	Id *string `json:"id"`
}

// profileReadRow is a lenient, provider-local view of one LAN-profile list
// entry. Only the fields the resource cares about are named; everything else is
// ignored by the decoder.
type profileReadRow struct {
	Id                   *string  `json:"id"`
	Name                 string   `json:"name"`
	NativeNetworkId      *string  `json:"nativeNetworkId"`
	TagNetworkIds        []string `json:"tagNetworkIds"`
	UntagNetworkIds      []string `json:"untagNetworkIds"`
	Poe                  int32    `json:"poe"`
	Dot1x                int32    `json:"dot1x"`
	BandWidthCtrlType    int32    `json:"bandWidthCtrlType"`
	PortIsolationEnable  bool     `json:"portIsolationEnable"`
	LldpMedEnable        bool     `json:"lldpMedEnable"`
	LoopbackDetectEnable bool     `json:"loopbackDetectEnable"`
	SpanningTreeEnable   bool     `json:"spanningTreeEnable"`
	Flag                 *int32   `json:"flag"`
	Type                 *int32   `json:"type"`
}

// listResult is the paged list payload { data: [ ... ] }.
type listResult struct {
	Data []profileReadRow `json:"data"`
}
