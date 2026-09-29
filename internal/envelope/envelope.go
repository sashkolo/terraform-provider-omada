// Package envelope decodes the Omada Open API's standard response envelope,
// {errorCode, msg, result}, for every resource in this provider.
//
// The generated SDK decodes response bodies with DisallowUnknownFields and
// enforces every required property, which rejects fields the controller returns
// that the SDK model does not know about. Resources therefore use the SDK only
// for the authenticated HTTP transport and decode the (re-readable) response
// body here with the standard library, which ignores unknown fields.
//
// Decode is deliberately strict about what counts as success: the Open API
// always answers with an errorCode, so a body without one, or any HTTP error
// status, is an error even when the body parses. Treating either as success
// once let a failed DELETE report success and drop a live object from state.
package envelope

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// Envelope is the standard {errorCode, msg, result} wrapper returned by every
// Open API endpoint. Result is captured raw and decoded per call.
type Envelope struct {
	ErrorCode *int32          `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// HasError reports a controller-side error (a non-zero errorCode). Decode only
// returns envelopes whose errorCode is present, so the nil check is defensive.
func (e Envelope) HasError() bool {
	return e.ErrorCode != nil && *e.ErrorCode != 0
}

// Code returns the errorCode, or 0 when it is absent.
func (e Envelope) Code() int32 {
	if e.ErrorCode == nil {
		return 0
	}
	return *e.ErrorCode
}

// Decode reads and decodes the envelope from an SDK call's response.
//
// It adds an error diagnostic and returns ok=false when:
//   - the call failed with no response (a transport error);
//   - the response has no body, or the body can't be read or parsed as JSON;
//   - the body has no errorCode, whatever the HTTP status (a proxy or gateway
//     error page shaped as JSON, or an endpoint that isn't the Open API);
//   - the HTTP status is 300 or above but errorCode is 0 (an inconsistent
//     answer).
//
// A present, non-zero errorCode is returned with ok=true, whatever the HTTP
// status, so callers can branch on specific controller codes (for example a
// not-found code on Delete) via HasError and Code.
func Decode(httpResp *http.Response, callErr error, diags *diag.Diagnostics, action string) (Envelope, bool) {
	if callErr != nil && httpResp == nil {
		diags.AddError("Error "+action, "Transport error: "+callErr.Error())
		return Envelope{}, false
	}
	if httpResp == nil {
		diags.AddError("Error "+action, "Controller returned no response.")
		return Envelope{}, false
	}
	// net/http guarantees a non-nil Body for a non-nil Response, but a mocked
	// transport or an SDK anomaly could return one without; guard the read.
	if httpResp.Body == nil {
		diags.AddError("Error "+action, "Controller returned a response with no body"+status(httpResp)+".")
		return Envelope{}, false
	}
	// The SDK drains the body and re-wraps it in a NopCloser, so Close is a
	// defensive no-op kept for robustness against SDK changes.
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		diags.AddError("Error "+action, "Could not read response body"+status(httpResp)+": "+err.Error())
		return Envelope{}, false
	}

	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		diags.AddError("Error "+action, "Could not decode response"+status(httpResp)+": "+err.Error()+withCallErr(callErr))
		return Envelope{}, false
	}

	if env.ErrorCode == nil {
		diags.AddError("Error "+action,
			"Controller response has no errorCode"+status(httpResp)+"; it is not an Open API answer, so the "+
				"operation's outcome is unknown."+withCallErr(callErr))
		return Envelope{}, false
	}

	// Only the HTTP status decides this, not callErr: the SDK also returns an
	// error for a 2xx body its strict models reject (an unknown or missing
	// field), which is exactly the drift the lenient decode exists to tolerate.
	if *env.ErrorCode == 0 && httpResp.StatusCode >= 300 {
		diags.AddError("Error "+action,
			"Controller returned errorCode 0 with an HTTP error"+status(httpResp)+", so the operation's "+
				"outcome is unknown."+withCallErr(callErr))
		return Envelope{}, false
	}

	return env, true
}

// AddAPIError adds the standard diagnostic for a controller-side error
// (a non-zero errorCode and its msg).
func AddAPIError(diags *diag.Diagnostics, action string, code *int32, msg string) {
	if code == nil {
		diags.AddError("Error "+action, "Controller rejected the request: "+msg)
		return
	}
	diags.AddError("Error "+action, fmt.Sprintf("Controller rejected the request, error code %d: %s", *code, msg))
}

func status(r *http.Response) string {
	if r == nil || r.StatusCode == 0 {
		return ""
	}
	return fmt.Sprintf(" (HTTP %d)", r.StatusCode)
}

func withCallErr(callErr error) string {
	if callErr == nil {
		return ""
	}
	return " (original error: " + callErr.Error() + ")"
}
