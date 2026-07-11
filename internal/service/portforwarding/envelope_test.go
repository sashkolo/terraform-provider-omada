package portforwarding

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func TestDecodeEnvelopeSurfacesErrorCodeLessTransportError(t *testing.T) {
	response := &http.Response{Body: io.NopCloser(strings.NewReader(`{"message":"bad gateway"}`))}
	var diagnostics diag.Diagnostics
	_, ok := decodeEnvelope(response, errors.New("502 Bad Gateway"), &diagnostics, "reading port forwarding")
	if ok {
		t.Fatal("decodeEnvelope unexpectedly accepted a transport error")
	}
	if !diagnostics.HasError() {
		t.Fatal("decodeEnvelope did not add an error diagnostic")
	}
}
