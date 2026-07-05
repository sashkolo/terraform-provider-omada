package ipgroup_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_IPGroupResource exercises full CRUD + import for the omada_ip_group
// resource against an httptest stand-in for the Omada Open API. The per-type
// group list endpoint returns the groups as a bare array under `result` (no
// {data} paging wrapper); the modify handler mutates the row in place so the
// subsequent Read reflects the change, exactly like the live controller. The
// create endpoint returns the new id under {id} (ResIdOpenApiVO).
func TestAcc_IPGroupResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// listRow is the single row returned by the IP-group list endpoint. It models
	// a two-entry group: a /32 host and a /24 subnet.
	// The controller does NOT echo the top-level group description in the
	// per-type list read (only name/ipList/portList), so it is intentionally
	// omitted here — the resource must preserve the configured description rather
	// than null it. The nested ipList description *is* returned.
	listRow := map[string]any{
		"groupId": "test-group-id",
		"name":    "vigi-cameras",
		"type":    int32(0),
		"ipList": []any{
			map[string]any{"ip": "192.168.30.51", "mask": int32(32), "description": "cam-porch"},
			map[string]any{"ip": "192.168.30.0", "mask": int32(24)},
		},
	}

	listResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "",
			"result":    []any{listRow},
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Create (POST .../profiles/groups): returns the new id under {id}.
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success.","result":{"id":"test-group-id"}}`)
	})

	// Read (GET .../profiles/groups/{groupType}, bare array result).
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, listResponse())
	})

	// Update (PATCH .../profiles/groups/{groupType}/{groupId}): reflect the name
	// change into the list row.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}/{groupId}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name != "" {
			listRow["name"] = req.Name
		}
		writeJSON(w, emptyResponse)
	})

	// Delete (DELETE .../profiles/groups/{groupType}/{groupId}).
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}/{groupId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: ts.ProviderConfig + `
				resource "omada_ip_group" "test" {
					site_id     = "test-site-id"
					name        = "vigi-cameras"
					description = "VIGI camera hosts"
					ip_list = [
						{ ip = "192.168.30.51", mask = 32, description = "cam-porch" },
						{ ip = "192.168.30.0", mask = 24 },
					]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ip_group.test", "group_id", "test-group-id"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "name", "vigi-cameras"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "description", "VIGI camera hosts"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.#", "2"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.0.ip", "192.168.30.51"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.0.mask", "32"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.0.description", "cam-porch"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.1.ip", "192.168.30.0"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.1.mask", "24"),
				),
			},
			// Import (ID format: <site_id>/<group_id>)
			{
				ResourceName:                         "omada_ip_group.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/test-group-id",
				ImportStateVerifyIdentifierAttribute: "group_id",
				// description is not returned by the controller's list read, so it
				// cannot be recovered on a bare import.
				ImportStateVerifyIgnore: []string{"description"},
			},
			// Update the name in place.
			{
				Config: ts.ProviderConfig + `
				resource "omada_ip_group" "test" {
					site_id     = "test-site-id"
					name        = "vigi-cameras-2"
					description = "VIGI camera hosts"
					ip_list = [
						{ ip = "192.168.30.51", mask = 32, description = "cam-porch" },
						{ ip = "192.168.30.0", mask = 24 },
					]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ip_group.test", "name", "vigi-cameras-2"),
				),
			},
		},
	})
}

// TestAcc_IPGroupResource_CreateWithoutId covers the create path when the
// controller omits the id in the create response (returns an empty envelope):
// the resource must fall back to a name-based list lookup to recover group_id.
// Because group_id is Computed it is Unknown (not Null) at create, so the
// fallback guard must treat Unknown as "not yet known" — a plain IsNull() guard
// would skip the lookup and leave the id unset.
func TestAcc_IPGroupResource_CreateWithoutId(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	listRow := map[string]any{
		"groupId": "recovered-group-id",
		"name":    "vigi-cameras",
		"type":    int32(0),
		"ipList": []any{
			map[string]any{"ip": "192.168.30.0", "mask": int32(24)},
		},
	}
	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Create returns success but no result/id, forcing the name-based recovery.
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success."}`)
	})
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}", func(w http.ResponseWriter, _ *http.Request) {
		b, _ := json.Marshal(map[string]any{"errorCode": 0, "msg": "", "result": []any{listRow}})
		writeJSON(w, string(b))
	})
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}/{groupId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{ "errorCode": 0, "msg": "" }`)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + `
				resource "omada_ip_group" "test" {
					site_id = "test-site-id"
					name    = "vigi-cameras"
					ip_list = [
						{ ip = "192.168.30.0", mask = 24 },
					]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ip_group.test", "group_id", "recovered-group-id"),
				),
			},
		},
	})
}
