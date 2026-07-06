package apwlangroup_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_ApWlanGroupResource exercises Create(switch)/Read/Update/Import for the
// omada_ap_wlan_group resource against an httptest stand-in for the Omada Open
// API. An AP always exists and is always bound to exactly one WLAN group, so
// there is no create/delete endpoint: Create/Update PATCH the per-AP wlan-group,
// and Read fetches the AP overview.
//
// The AP overview row is mutated in place by the switch handler so the
// subsequent Read reflects the change. The switch handler also rejects a switch
// to the AP's current group (as the real controller does — "cannot be the
// current wlan group"), which is the regression guard for the provider's no-op
// switch skip: a redundant PATCH would fail the apply.
func TestAcc_ApWlanGroupResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// apRow is the mutable AP overview. The AP starts on "default-group"; note
	// the literal "wlan group id" key (with spaces) the controller returns.
	apRow := map[string]any{
		"mac":           "A4-2B-B0-11-22-33",
		"name":          "AP Test",
		"model":         "EAP653",
		"wlan group id": "default-group",
	}

	overviewResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "",
			"result":    apRow,
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "Success." }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Switch (PATCH .../aps/{apMac}/wlan-group): decode the body, reject a switch
	// to the current group (controller behavior + no-op-skip regression guard),
	// otherwise reflect the new group into the AP row.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/aps/{apMac}/wlan-group", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			WlanGroupId string `json:"wlanGroupId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.WlanGroupId == "" {
			writeJSON(w, `{"errorCode":-1,"msg":"wlanGroupId is required"}`)
			return
		}
		if body.WlanGroupId == apRow["wlan group id"] {
			writeJSON(w, `{"errorCode":-1,"msg":"cannot switch to the current wlan group"}`)
			return
		}
		apRow["wlan group id"] = body.WlanGroupId
		writeJSON(w, emptyResponse)
	})

	// Read (GET .../aps/{apMac}): AP overview.
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/aps/{apMac}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, overviewResponse())
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create where the AP is already on the desired group: the provider
			// must skip the switch PATCH (which the controller would reject) and
			// still succeed, proving the no-op guard and idempotent adoption.
			{
				Config: ts.ProviderConfig + `
				resource "omada_ap_wlan_group" "test" {
					site_id       = "test-site-id"
					ap_mac        = "A4-2B-B0-11-22-33"
					wlan_group_id = "default-group"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "ap_mac", "A4-2B-B0-11-22-33"),
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "wlan_group_id", "default-group"),
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "ap_name", "AP Test"),
				),
			},
			// Import (ID format: <site_id>/<ap_mac>).
			{
				ResourceName:                         "omada_ap_wlan_group.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/A4-2B-B0-11-22-33",
				ImportStateVerifyIdentifierAttribute: "wlan_group_id",
			},
			// Update: switch the AP to a different group.
			{
				Config: ts.ProviderConfig + `
				resource "omada_ap_wlan_group" "test" {
					site_id       = "test-site-id"
					ap_mac        = "A4-2B-B0-11-22-33"
					wlan_group_id = "staging-group"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "wlan_group_id", "staging-group"),
				),
			},
		},
	})
}
