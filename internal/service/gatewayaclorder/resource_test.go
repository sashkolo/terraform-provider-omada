package gatewayaclorder_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_GatewayAclOrderResource exercises create/read/import/update for the
// omada_gateway_acl_order resource against an httptest stand-in. A mutable
// `order` slice models the live gateway ACL order: GET returns rows with a
// 1-based index reflecting the current order; POST /acls/modifyIndex applies the
// {id: index} map so subsequent reads reflect the new order — exactly like the
// controller.
func TestAcc_GatewayAclOrderResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	var mu sync.Mutex
	// Initial live order.
	order := []string{"acl-a", "acl-b", "acl-c"}

	writeJSON := func(w http.ResponseWriter, body any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}

	// GET gateway ACL list: rows carry the 1-based index of their position.
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		data := make([]map[string]any, 0, len(order))
		for i, id := range order {
			data = append(data, map[string]any{"id": id, "index": int32(i + 1)})
		}
		writeJSON(w, map[string]any{
			"errorCode": 0, "msg": "",
			"result": map[string]any{"totalRows": len(data), "currentPage": 1, "currentSize": 1000, "data": data},
		})
	})

	// POST /acls/modifyIndex: reorder `order` by the sent {id: index} map.
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/acls/modifyIndex", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Indexes map[string]int32 `json:"indexes"`
			Type    string           `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		next := make([]string, 0, len(body.Indexes))
		for id := range body.Indexes {
			next = append(next, id)
		}
		sort.SliceStable(next, func(i, j int) bool { return body.Indexes[next[i]] < body.Indexes[next[j]] })
		order = next
		mu.Unlock()
		writeJSON(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create: reorder to c, a, b.
			{
				Config: ts.ProviderConfig + `
				resource "omada_gateway_acl_order" "test" {
					site_id         = "test-site-id"
					ordered_acl_ids = ["acl-c", "acl-a", "acl-b"]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gateway_acl_order.test", "ordered_acl_ids.#", "3"),
					resource.TestCheckResourceAttr("omada_gateway_acl_order.test", "ordered_acl_ids.0", "acl-c"),
					resource.TestCheckResourceAttr("omada_gateway_acl_order.test", "ordered_acl_ids.1", "acl-a"),
					resource.TestCheckResourceAttr("omada_gateway_acl_order.test", "ordered_acl_ids.2", "acl-b"),
				),
			},
			// Import by site_id; Read reflects the live order (c, a, b).
			{
				ResourceName:                         "omada_gateway_acl_order.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id",
				ImportStateVerifyIdentifierAttribute: "site_id",
			},
			// Update: reorder to a, b, c.
			{
				Config: ts.ProviderConfig + `
				resource "omada_gateway_acl_order" "test" {
					site_id         = "test-site-id"
					ordered_acl_ids = ["acl-a", "acl-b", "acl-c"]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gateway_acl_order.test", "ordered_acl_ids.0", "acl-a"),
					resource.TestCheckResourceAttr("omada_gateway_acl_order.test", "ordered_acl_ids.2", "acl-c"),
				),
			},
			// A non-exhaustive list (missing acl-c) is rejected, not silently
			// applied — this is what prevents clobbering out-of-band rules.
			{
				Config: ts.ProviderConfig + `
				resource "omada_gateway_acl_order" "test" {
					site_id         = "test-site-id"
					ordered_acl_ids = ["acl-a", "acl-b"]
				}
				`,
				ExpectError: regexp.MustCompile(`not exhaustive|acl-c`),
			},
		},
	})
}
