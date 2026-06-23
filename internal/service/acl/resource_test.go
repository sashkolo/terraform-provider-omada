package acl_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_AclResource exercises full CRUD + import for the gateway (OSG) ACL
// resource against an httptest stand-in for the Omada Open API. The list row is
// mutated in place by the modify handler so the subsequent Read reflects the
// change, exactly like the live controller. The read-only `index` is returned
// by the controller and round-trips through state; it is never sent on write.
func TestAcc_AclResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// listRow is the single row returned by the paged gateway ACL list endpoint.
	listRow := map[string]any{
		"id":              "test-acl-id",
		"index":           int32(1),
		"description":     "TF Probe ACL",
		"sourceType":      int32(1), // IP Group
		"sourceIds":       []any{"ip-group-1"},
		"destinationType": int32(0), // network
		"destinationIds":  []any{"net-1"},
		"policy":          int32(1), // allow
		"protocols":       []any{int32(6)},
		"stateMode":       int32(0), // auto
		"status":          true,
		"syslog":          false,
		"direction": map[string]any{
			"lanToLan": true,
		},
	}

	listResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "",
			"result": map[string]any{
				"totalRows":   1,
				"currentPage": 1,
				"currentSize": 1000,
				"data":        []any{listRow},
			},
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Create (POST .../acls/osg-acls): the controller wraps the new id (aclId)
	// in the standard envelope.
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"","result":{"aclId":"test-acl-id"}}`)
	})

	// Read (GET .../acls/osg-acls, paged list).
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, listResponse())
	})

	// Update (PUT .../acls/osg-acls/{aclId}): decode the body and reflect the
	// description change into the list row.
	mux.HandleFunc("PUT /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls/{aclId}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Description != "" {
			listRow["description"] = req.Description
		}

		writeJSON(w, emptyResponse)
	})

	// Delete (DELETE .../acls/{aclId}) — the generic, device-type-agnostic path.
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/acls/{aclId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read. destination_ids is computed (the controller resolves
			// it for the network destination type); the read-only index is
			// controller-assigned.
			{
				Config: ts.ProviderConfig + `
				resource "omada_acl" "test" {
					site_id          = "test-site-id"
					description      = "TF Probe ACL"
					source_type      = 1
					source_ids       = ["ip-group-1"]
					destination_type = 0
					policy           = 1
					protocols        = [6]
					state_mode       = 0
					status           = true
					syslog           = false

					direction = {
						lan_to_lan = true
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_acl.test", "acl_id", "test-acl-id"),
					resource.TestCheckResourceAttr("omada_acl.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_acl.test", "index", "1"),
					resource.TestCheckResourceAttr("omada_acl.test", "description", "TF Probe ACL"),
					resource.TestCheckResourceAttr("omada_acl.test", "source_type", "1"),
					resource.TestCheckResourceAttr("omada_acl.test", "source_ids.#", "1"),
					resource.TestCheckResourceAttr("omada_acl.test", "source_ids.0", "ip-group-1"),
					resource.TestCheckResourceAttr("omada_acl.test", "destination_type", "0"),
					resource.TestCheckResourceAttr("omada_acl.test", "destination_ids.#", "1"),
					resource.TestCheckResourceAttr("omada_acl.test", "destination_ids.0", "net-1"),
					resource.TestCheckResourceAttr("omada_acl.test", "policy", "1"),
					resource.TestCheckResourceAttr("omada_acl.test", "protocols.#", "1"),
					resource.TestCheckResourceAttr("omada_acl.test", "protocols.0", "6"),
					resource.TestCheckResourceAttr("omada_acl.test", "state_mode", "0"),
					resource.TestCheckResourceAttr("omada_acl.test", "status", "true"),
					resource.TestCheckResourceAttr("omada_acl.test", "syslog", "false"),
					resource.TestCheckResourceAttr("omada_acl.test", "direction.lan_to_lan", "true"),
				),
			},
			// Import (ID format: <site_id>/<acl_id>)
			{
				ResourceName:                         "omada_acl.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/test-acl-id",
				ImportStateVerifyIdentifierAttribute: "acl_id",
			},
			// Update (description) + Read
			{
				Config: ts.ProviderConfig + `
				resource "omada_acl" "test" {
					site_id          = "test-site-id"
					description      = "TF Probe ACL Renamed"
					source_type      = 1
					source_ids       = ["ip-group-1"]
					destination_type = 0
					policy           = 1
					protocols        = [6]
					state_mode       = 0
					status           = true
					syslog           = false

					direction = {
						lan_to_lan = true
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_acl.test", "description", "TF Probe ACL Renamed"),
					resource.TestCheckResourceAttr("omada_acl.test", "acl_id", "test-acl-id"),
					resource.TestCheckResourceAttr("omada_acl.test", "index", "1"),
				),
			},
			// Delete is exercised automatically by the test harness.
		},
	})
}
