package gatewayaclorder

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope. Mirrors
// internal/service/acl/envelope.go: the generated SDK decodes response bodies
// with DisallowUnknownFields, which rejects controller fields the SDK model does
// not know. These helpers decode the re-readable body with the standard library
// instead.

// omadaEnvelope is the standard {errorCode, msg, result} wrapper.
type omadaEnvelope struct {
	ErrorCode *int32          `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// hasError reports a controller-side error (non-zero errorCode).
func (e omadaEnvelope) hasError() bool {
	return e.ErrorCode != nil && *e.ErrorCode != 0
}

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
