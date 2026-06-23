package wlangroup_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAcc_WlanGroupResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// rows is the list returned by GET /wlans. The controller's v1 list returns
	// a bare array under "result"; rename via PATCH mutates the in-memory row
	// so the subsequent Read reflects the update, exactly like the live API.
	rows := []map[string]any{
		{
			"wlanId":  "default-group-id",
			"name":    "Default",
			"primary": true,
		},
		{
			"wlanId":  "test-wlan-group-id",
			"name":    "TF Probe WLAN Group",
			"primary": false,
		},
	}

	listResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "Success.",
			"result":    rows,
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Create (POST /wlans): the controller wraps the new id in the standard
	// envelope.
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"","result":{"id":"test-wlan-group-id"}}`)
	})

	// Read (GET /wlans): bare-array list.
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, listResponse())
	})

	// Update (PATCH /wlans/{wlanId}): rename only.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans/{wlanId}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			for i := range rows {
				if rows[i]["wlanId"] == "test-wlan-group-id" {
					rows[i]["name"] = req.Name
				}
			}
		}

		writeJSON(w, emptyResponse)
	})

	// Delete
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans/{wlanId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read
			{
				Config: ts.ProviderConfig + `
				resource "omada_wlan_group" "test" {
					site_id = "test-site-id"
					name    = "TF Probe WLAN Group"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_wlan_group.test", "wlan_group_id", "test-wlan-group-id"),
					resource.TestCheckResourceAttr("omada_wlan_group.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_wlan_group.test", "name", "TF Probe WLAN Group"),
					resource.TestCheckResourceAttr("omada_wlan_group.test", "primary", "false"),
				),
			},
			// Import (ID format: <site_id>/<wlan_group_id>)
			{
				ResourceName:                         "omada_wlan_group.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/test-wlan-group-id",
				ImportStateVerifyIdentifierAttribute: "wlan_group_id",
			},
			// Update (rename) + Read
			{
				Config: ts.ProviderConfig + `
				resource "omada_wlan_group" "test" {
					site_id = "test-site-id"
					name    = "TF Probe WLAN Group Renamed"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_wlan_group.test", "name", "TF Probe WLAN Group Renamed"),
					resource.TestCheckResourceAttr("omada_wlan_group.test", "wlan_group_id", "test-wlan-group-id"),
				),
			},
			// Delete is exercised automatically by the test harness.
		},
	})
}
