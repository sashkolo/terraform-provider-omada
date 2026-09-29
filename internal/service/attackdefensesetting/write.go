package attackdefensesetting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"terraform-provider-omada/internal/envelope"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// The write path is built by hand rather than through the generated SDK model.
// The controller returns AND expects "securityEnable" for the IP-security-option
// toggle, but the SDK's SpecifiedOptionOpenApiVO hardcodes the codegen name
// "securityOptionEnable" in its JSON marshaling with no override hook. Sending
// the SDK key silently no-ops that toggle on the controller (Modify accepts the
// body but ignores the unknown field), which would leave the resource in
// perpetual drift: the plan sets security_option_enable, the PATCH never applies
// it, and the next read reports the unchanged controller value. The read side
// already compensates for the same key drift (see envelope.go, fork v0.7.4), so
// the write side mirrors it: build the modify body with the controller's keys
// and PATCH it through the SDK's configured transport (base URL, auth header,
// and TLS client), matching the lenient read in resource.go.

// specifiedOptionWriteVO is the modify-body shape for the nested IP-option block.
// Every field is an optional pointer (emitted only when set), matching the SDK's
// omitempty marshaling; the sole deviation is SecurityEnable's json key.
type specifiedOptionWriteVO struct {
	NoOperationEnable *bool `json:"noOperationEnable,omitempty"`
	RecordRouteEnable *bool `json:"recordRouteEnable,omitempty"`
	SecurityEnable    *bool `json:"securityEnable,omitempty"`
	StreamEnable      *bool `json:"streamEnable,omitempty"`
	TimestampEnable   *bool `json:"timestampEnable,omitempty"`
}

// attackDefenseWriteVO is the full modify body. It mirrors the SDK
// AttackDefenseSetting wire format field-for-field — required enable toggles are
// always sent; limits, thresholds, rejects, and the nested block are omitempty —
// so the only behavioral change from the prior SDK-driven write is the corrected
// specifiedOption.securityEnable key.
type attackDefenseWriteVO struct {
	IcmpConnEnable             bool                    `json:"icmpConnEnable"`
	IcmpConnLimit              *int32                  `json:"icmpConnLimit,omitempty"`
	IcmpSrcEnable              bool                    `json:"icmpSrcEnable"`
	IcmpSrcLimit               *int32                  `json:"icmpSrcLimit,omitempty"`
	IcmpTimestampRequestReject *bool                   `json:"icmpTimestampRequestReject,omitempty"`
	LargePingEnable            bool                    `json:"largePingEnable"`
	LargePingThreshold         *int32                  `json:"largePingThreshold,omitempty"`
	PingDeathEnable            bool                    `json:"pingDeathEnable"`
	PingWanEnable              bool                    `json:"pingWanEnable"`
	SpecifiedOptionEnable      bool                    `json:"specifiedOptionEnable"`
	SpecifiedOption            *specifiedOptionWriteVO `json:"specifiedOption,omitempty"`
	TcpConnEnable              bool                    `json:"tcpConnEnable"`
	TcpConnLimit               *int32                  `json:"tcpConnLimit,omitempty"`
	TcpFinNoAckEnable          bool                    `json:"tcpFinNoAckEnable"`
	TcpScanEnable              bool                    `json:"tcpScanEnable"`
	TcpScanReject              *bool                   `json:"tcpScanReject,omitempty"`
	TcpSrcEnable               bool                    `json:"tcpSrcEnable"`
	TcpSrcLimit                *int32                  `json:"tcpSrcLimit,omitempty"`
	TcpSynFinEnable            bool                    `json:"tcpSynFinEnable"`
	UdpConnEnable              bool                    `json:"udpConnEnable"`
	UdpConnLimit               *int32                  `json:"udpConnLimit,omitempty"`
	UdpSrcEnable               bool                    `json:"udpSrcEnable"`
	UdpSrcLimit                *int32                  `json:"udpSrcLimit,omitempty"`
	WinNukeAttackEnable        bool                    `json:"winNukeAttackEnable"`
}

// buildSpecifiedOptionWrite converts the optional Terraform block into the
// modify VO. Returns nil when the block is absent.
func buildSpecifiedOptionWrite(s *specifiedOptionModel) *specifiedOptionWriteVO {
	if s == nil {
		return nil
	}
	return &specifiedOptionWriteVO{
		NoOperationEnable: s.NoOperationEnable.ValueBoolPointer(),
		RecordRouteEnable: s.RecordRouteEnable.ValueBoolPointer(),
		SecurityEnable:    s.SecurityOptionEnable.ValueBoolPointer(),
		StreamEnable:      s.StreamEnable.ValueBoolPointer(),
		TimestampEnable:   s.TimestampEnable.ValueBoolPointer(),
	}
}

// buildAttackDefenseWriteBody builds the modify body sent on Create and Update.
// Optional (pointer) fields are sent only when set; the whole object is sent on
// every write (it is a coarse blob).
func buildAttackDefenseWriteBody(plan attackDefenseSettingResourceModel) attackDefenseWriteVO {
	return attackDefenseWriteVO{
		IcmpConnEnable:             plan.IcmpConnEnable.ValueBool(),
		IcmpConnLimit:              plan.IcmpConnLimit.ValueInt32Pointer(),
		IcmpSrcEnable:              plan.IcmpSrcEnable.ValueBool(),
		IcmpSrcLimit:               plan.IcmpSrcLimit.ValueInt32Pointer(),
		IcmpTimestampRequestReject: plan.IcmpTimestampRequestReject.ValueBoolPointer(),
		LargePingEnable:            plan.LargePingEnable.ValueBool(),
		LargePingThreshold:         plan.LargePingThreshold.ValueInt32Pointer(),
		PingDeathEnable:            plan.PingDeathEnable.ValueBool(),
		PingWanEnable:              plan.PingWanEnable.ValueBool(),
		SpecifiedOptionEnable:      plan.SpecifiedOptionEnable.ValueBool(),
		SpecifiedOption:            buildSpecifiedOptionWrite(plan.SpecifiedOption),
		TcpConnEnable:              plan.TcpConnEnable.ValueBool(),
		TcpConnLimit:               plan.TcpConnLimit.ValueInt32Pointer(),
		TcpFinNoAckEnable:          plan.TcpFinNoAckEnable.ValueBool(),
		TcpScanEnable:              plan.TcpScanEnable.ValueBool(),
		TcpScanReject:              plan.TcpScanReject.ValueBoolPointer(),
		TcpSrcEnable:               plan.TcpSrcEnable.ValueBool(),
		TcpSrcLimit:                plan.TcpSrcLimit.ValueInt32Pointer(),
		TcpSynFinEnable:            plan.TcpSynFinEnable.ValueBool(),
		UdpConnEnable:              plan.UdpConnEnable.ValueBool(),
		UdpConnLimit:               plan.UdpConnLimit.ValueInt32Pointer(),
		UdpSrcEnable:               plan.UdpSrcEnable.ValueBool(),
		UdpSrcLimit:                plan.UdpSrcLimit.ValueInt32Pointer(),
		WinNukeAttackEnable:        plan.WinNukeAttackEnable.ValueBool(),
	}
}

// modifyAttackDefenseSetting PATCHes the whole settings object to the controller
// using the SDK client's configured transport (server URL, auth header, TLS
// client), then leniently decodes the standard Omada envelope. It returns false
// only on a controller/transport error (diagnostics are populated in that case).
func modifyAttackDefenseSetting(ctx context.Context, r *attackDefenseSettingResource, diags *diag.Diagnostics, action string, plan attackDefenseSettingResourceModel) bool {
	body, err := json.Marshal(buildAttackDefenseWriteBody(plan))
	if err != nil {
		diags.AddError("Error "+action, "Could not encode settings: "+err.Error())
		return false
	}

	if r.client == nil {
		diags.AddError("Error "+action, "Provider client is not configured.")
		return false
	}
	cfg := r.client.GetConfig()
	if cfg == nil || len(cfg.Servers) == 0 || cfg.Servers[0].URL == "" {
		diags.AddError("Error "+action, "Provider client has no server URL configured.")
		return false
	}
	url := fmt.Sprintf("%s/openapi/v1/%s/sites/%s/attack-defense", cfg.Servers[0].URL, r.omadacId, plan.SiteId.ValueString())

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		diags.AddError("Error "+action, "Could not build request: "+err.Error())
		return false
	}
	// Carry every default header the SDK/provider configured, then set the
	// request-specific content type. The access token is not among them: the
	// client's transport adds (and renews) it on every request, this one
	// included, because it goes through cfg.HTTPClient.
	for k, v := range cfg.DefaultHeader {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	// http.Client.Do only errors on transport/redirect-policy failures (not on
	// non-2xx), and on a redirect-policy failure the returned body is already
	// closed. Handle the error up front rather than letting envelope.Decode read a
	// closed body; a controller-side rejection arrives as a 200 envelope instead.
	httpResp, callErr := httpClient.Do(req)
	if callErr != nil {
		diags.AddError("Error "+action, "Transport error: "+callErr.Error())
		return false
	}
	env, ok := envelope.Decode(httpResp, nil, diags, action)
	if !ok {
		return false
	}
	if env.HasError() {
		envelope.AddAPIError(diags, action, env.ErrorCode, env.Msg)
		return false
	}
	return true
}
