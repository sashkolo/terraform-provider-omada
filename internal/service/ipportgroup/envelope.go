package ipportgroup

import "encoding/json"

// Local, lenient decode types for the Omada Open API standard envelope. Mirrors
// internal/service/ipgroup/envelope.go and the other homelab resources: the
// generated SDK decodes response bodies with DisallowUnknownFields, which
// rejects controller fields the SDK model does not know. These helpers decode
// the re-readable body with the standard library instead.

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

// createResult is the {id} payload of the create endpoint (ResIdOpenApiVO).
type createResult struct {
	Id *string `json:"id"`
}

// ipSubnetReadVO mirrors the controller's ipList entry on group reads.
type ipSubnetReadVO struct {
	Ip          string  `json:"ip"`
	Mask        int32   `json:"mask"`
	Description *string `json:"description,omitempty"`
}

// portMaskReadVO mirrors the controller's portMaskList entry.
type portMaskReadVO struct {
	Port int32  `json:"port"`
	Mask string `json:"mask"`
}

// groupReadRow is a lenient, provider-local view of one group-profile list
// entry. Only the fields an IP-Port group cares about are named; everything
// else is ignored by the decoder.
type groupReadRow struct {
	GroupId      *string          `json:"groupId"`
	Name         string           `json:"name"`
	Description  *string          `json:"description,omitempty"`
	Type         *int32           `json:"type,omitempty"`
	IpList       []ipSubnetReadVO `json:"ipList,omitempty"`
	PortType     *int32           `json:"portType,omitempty"`
	PortList     []string         `json:"portList,omitempty"`
	PortMaskList []portMaskReadVO `json:"portMaskList,omitempty"`
}
