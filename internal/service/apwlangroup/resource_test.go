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

	// 24-hex Omada object ids for the WLAN groups. The provider accepts only
	// id-shaped values, so a group name would (correctly) not be read back.
	const defaultGroup = "638ef75a73919c1e1734f77a"
	const stagingGroup = "638ef75a73919c1e1734f88b"

	// apRow is the mutable AP overview. Note the group is keyed under the
	// camelCase "wlanGroupId" (not the SDK's spaced "wlan group id") — the
	// provider's multi-key scan must still find it. The AP starts on the default
	// group.
	apRow := map[string]any{
		"mac":         "A4-2B-B0-11-22-33",
		"name":        "AP Test",
		"model":       "EAP653",
		"wlanGroupId": defaultGroup,
	}

	// apGone flips the overview read to the controller's not-found error so the
	// resource's drop-from-state path can be exercised.
	var apGone bool

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
		if body.WlanGroupId == apRow["wlanGroupId"] {
			writeJSON(w, `{"errorCode":-1,"msg":"cannot switch to the current wlan group"}`)
			return
		}
		apRow["wlanGroupId"] = body.WlanGroupId
		writeJSON(w, emptyResponse)
	})

	// Read (GET .../aps/{apMac}): AP overview, or the controller's not-found
	// error (-1001) once the AP has been "forgotten".
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/aps/{apMac}", func(w http.ResponseWriter, _ *http.Request) {
		if apGone {
			writeJSON(w, `{"errorCode":-1001,"msg":"invalid request parameters"}`)
			return
		}
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
					wlan_group_id = "638ef75a73919c1e1734f77a"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "ap_mac", "A4-2B-B0-11-22-33"),
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "wlan_group_id", "638ef75a73919c1e1734f77a"),
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
					wlan_group_id = "638ef75a73919c1e1734f88b"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ap_wlan_group.test", "wlan_group_id", "638ef75a73919c1e1734f88b"),
				),
			},
			// Drift: the AP is forgotten upstream (overview read returns -1001).
			// Read must drop the resource from state, so a refresh-only plan is
			// non-empty (Terraform wants to recreate the binding).
			{
				PreConfig:          func() { apGone = true },
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
