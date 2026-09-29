package gatewayaclorder

// Lenient, provider-local decode types for this resource's result payloads.
// The {errorCode, msg, result} envelope itself is decoded by
// internal/envelope, which also explains why these types exist: the SDK's
// strict models reject fields the controller returns that they don't know.

// aclIndexRow is a lenient view of one gateway ACL list entry. Only the id and
// its controller-assigned order index are needed to observe/steer ordering;
// every other field is ignored by the decoder.
type aclIndexRow struct {
	Id    string `json:"id"`
	Index int32  `json:"index"`
}

// listResult is the paged gateway ACL list payload { data: [ ... ] }.
type listResult struct {
	Data []aclIndexRow `json:"data"`
}
