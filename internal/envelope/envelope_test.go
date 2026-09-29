package envelope

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestDecode(t *testing.T) {
	sdkStrictErr := errors.New("no value given for required property currentPage")

	cases := []struct {
		name     string
		resp     *http.Response
		callErr  error
		wantOK   bool
		wantCode int32
		wantMsg  string // substring of the diagnostic detail when !wantOK
	}{
		{
			name:    "transport error with no response",
			callErr: errors.New("dial tcp: connection refused"),
			wantMsg: "Transport error",
		},
		{
			name:    "no response and no error",
			wantMsg: "no response",
		},
		{
			name:    "response with no body",
			resp:    &http.Response{StatusCode: 200},
			wantMsg: "no body",
		},
		{
			name:    "body that is not JSON",
			resp:    response(200, "<html>gateway</html>"),
			wantMsg: "Could not decode",
		},
		{
			// A proxy's JSON error page on a failed DELETE was
			// treated as success, so destroy dropped a live rule from state.
			name:    "HTTP 500 with a JSON body and no errorCode",
			resp:    response(500, `{"timestamp":"2026-09-29","status":500,"error":"Internal Server Error"}`),
			callErr: errors.New("500 Internal Server Error"),
			wantMsg: "no errorCode (HTTP 500)",
		},
		{
			name:    "HTTP 200 with a JSON body and no errorCode",
			resp:    response(200, `{"result":{}}`),
			wantMsg: "no errorCode (HTTP 200)",
		},
		{
			name:    "HTTP 502 claiming errorCode 0",
			resp:    response(502, `{"errorCode":0,"msg":"Success."}`),
			callErr: errors.New("502 Bad Gateway"),
			wantMsg: "errorCode 0 with an HTTP error (HTTP 502)",
		},
		{
			// The SDK's strict models reject fields they don't know; the lenient
			// decode exists to accept exactly this.
			name:    "HTTP 200 success that the SDK's strict decode rejected",
			resp:    response(200, `{"errorCode":0,"msg":"Success.","result":{"data":[]}}`),
			callErr: sdkStrictErr,
			wantOK:  true,
		},
		{
			name:     "controller error on HTTP 200",
			resp:     response(200, `{"errorCode":-33006,"msg":"Duplicate description."}`),
			wantOK:   true,
			wantCode: -33006,
		},
		{
			name:     "controller error on an HTTP error status",
			resp:     response(400, `{"errorCode":-1001,"msg":"Invalid request parameters."}`),
			callErr:  errors.New("400 Bad Request"),
			wantOK:   true,
			wantCode: -1001,
		},
		{
			name:   "plain success",
			resp:   response(200, `{"errorCode":0,"msg":"Success.","result":{"id":"abc"}}`),
			wantOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			env, ok := Decode(tc.resp, tc.callErr, &diags, "testing")

			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (diagnostics: %v)", ok, tc.wantOK, diags)
			}
			if !ok {
				if !diags.HasError() {
					t.Fatal("rejected response added no error diagnostic")
				}
				if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, tc.wantMsg) {
					t.Fatalf("detail %q does not contain %q", detail, tc.wantMsg)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("accepted response added diagnostics: %v", diags)
			}
			if env.Code() != tc.wantCode {
				t.Fatalf("Code() = %d, want %d", env.Code(), tc.wantCode)
			}
			if env.HasError() != (tc.wantCode != 0) {
				t.Fatalf("HasError() = %v for code %d", env.HasError(), tc.wantCode)
			}
		})
	}
}

func TestAddAPIError(t *testing.T) {
	var diags diag.Diagnostics
	code := int32(-33006)
	AddAPIError(&diags, "creating ACL", &code, "Duplicate description.")
	got := diags.Errors()[0]
	if got.Summary() != "Error creating ACL" {
		t.Fatalf("summary = %q", got.Summary())
	}
	if want := "Controller rejected the request, error code -33006: Duplicate description."; got.Detail() != want {
		t.Fatalf("detail = %q, want %q", got.Detail(), want)
	}
}
