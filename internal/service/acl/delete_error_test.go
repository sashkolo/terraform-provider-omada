package acl_test

import (
	"net/http"
	"regexp"
	"sync/atomic"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_AclDeleteHTTPErrorWithoutErrorCode is the homelab #235 regression: a
// DELETE answered with HTTP 500 and a JSON error page that carries no errorCode
// (a Spring or reverse-proxy error body) used to count as success, so destroy
// dropped the rule from state while it stayed live and unmanaged. The destroy
// must fail instead, and succeed once the controller answers properly.
func TestAcc_AclDeleteHTTPErrorWithoutErrorCode(t *testing.T) {
	ts := acctest.NewTestServer(t)

	const row = `{"id":"acl-1","index":1,"description":"Probe deny","sourceType":0,"sourceIds":["net-a"],` +
		`"destinationType":0,"destinationIds":["net-b"],"policy":0,"protocols":[6],"stateMode":0,` +
		`"status":true,"syslog":false,"direction":{"lanToLan":true}}`

	var deleted, failDelete atomic.Bool

	writeJSON := func(w http.ResponseWriter, status int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}

	ts.Mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"errorCode":0,"msg":"Success."}`)
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls", func(w http.ResponseWriter, _ *http.Request) {
		data := "[" + row + "]"
		if deleted.Load() {
			data = "[]"
		}
		writeJSON(w, http.StatusOK, `{"errorCode":0,"msg":"","result":{"totalRows":1,"currentPage":1,"currentSize":1000,"data":`+data+`}}`)
	})
	ts.Mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/acls/{aclId}", func(w http.ResponseWriter, _ *http.Request) {
		if failDelete.Load() {
			writeJSON(w, http.StatusInternalServerError,
				`{"timestamp":"2026-09-29T10:00:00.000+00:00","status":500,"error":"Internal Server Error","path":"/openapi"}`)
			return
		}
		deleted.Store(true)
		writeJSON(w, http.StatusOK, `{"errorCode":0,"msg":"Success."}`)
	})

	config := ts.ProviderConfig + `
	resource "omada_acl" "test" {
		site_id          = "test-site-id"
		description      = "Probe deny"
		source_type      = 0
		source_ids       = ["net-a"]
		destination_type = 0
		destination_ids  = ["net-b"]
		policy           = 0
		protocols        = [6]
		state_mode       = 0
		status           = true
		syslog           = false
		direction = {
			lan_to_lan = true
		}
	}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				// Removing the resource from config runs Delete against the failing
				// controller: the apply must fail, keeping the rule in state.
				PreConfig:   func() { failDelete.Store(true) },
				Config:      ts.ProviderConfig,
				ExpectError: regexp.MustCompile(`no errorCode \(HTTP 500\)`),
			},
			{
				// The controller recovers; the same removal now deletes the rule.
				PreConfig: func() { failDelete.Store(false) },
				Config:    ts.ProviderConfig,
			},
		},
	})

	if !deleted.Load() {
		t.Fatal("the rule was never deleted after the controller recovered")
	}
}
