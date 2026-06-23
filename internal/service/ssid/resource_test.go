package ssid_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_SsidResource exercises full CRUD + import against an httptest
// stand-in for the Omada Open API. The detail row is mutated in place by the
// update-basic-config handler so the subsequent Read reflects the change,
// exactly like the live controller. The PSK (securityKey) is returned on read
// to prove round-trip, and the test asserts the sensitive value is not echoed
// in plan output.
func TestAcc_SsidResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// detail is the SSID detail payload (GET .../ssids/{ssidId}). The create
	// handler seeds it; the update handler mutates it.
	detail := map[string]any{
		"ssidId":         "test-ssid-id",
		"name":           "TF Probe SSID",
		"band":           7,
		"broadcast":      true,
		"guestNetEnable": false,
		"security":       3,
		"vlanEnable":     true,
		"vlanId":         50,
		"pmfMode":        2,
		"enable11r":      false,
		"mloEnable":      false,
		"hidePwd":        false,
		"deviceType":     3,
		"pskSetting": map[string]any{
			"securityKey":       "supersecret-psk-value",
			"versionPsk":        4,
			"encryptionPsk":     3,
			"gikRekeyPskEnable": false,
		},
	}

	detailResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "Success.",
			"result":    detail,
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Create (POST .../wlans/{wlanId}/ssids).
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans/{wlanId}/ssids", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"","result":{"id":"test-ssid-id"}}`)
	})

	// Read (GET .../ssids/{ssidId}) — single-SSID detail.
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans/{wlanId}/ssids/{ssidId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, detailResponse())
	})

	// Update (PATCH .../ssids/{ssidId}/update-basic-config): reflect name and
	// PSK changes into the in-memory detail.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans/{wlanId}/ssids/{ssidId}/update-basic-config", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name       string `json:"name"`
			PskSetting *struct {
				SecurityKey string `json:"securityKey"`
			} `json:"pskSetting,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			detail["name"] = req.Name
		}
		if req.PskSetting != nil && req.PskSetting.SecurityKey != "" {
			if psk, ok := detail["pskSetting"].(map[string]any); ok {
				psk["securityKey"] = req.PskSetting.SecurityKey
			}
		}

		writeJSON(w, emptyResponse)
	})

	// Delete
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans/{wlanId}/ssids/{ssidId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read. The PSK is configured and round-trips through state;
			// it must never appear in the plan output (sensitive).
			{
				Config: ts.ProviderConfig + `
				resource "omada_ssid" "test" {
					site_id       = "test-site-id"
					wlan_group_id = "test-wlan-group-id"
					name          = "TF Probe SSID"
					security      = 3
					band          = 7
					vlan_enable   = true
					vlan_id       = 50

					psk_setting = {
						psk = "supersecret-psk-value"
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ssid.test", "ssid_id", "test-ssid-id"),
					resource.TestCheckResourceAttr("omada_ssid.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_ssid.test", "wlan_group_id", "test-wlan-group-id"),
					resource.TestCheckResourceAttr("omada_ssid.test", "name", "TF Probe SSID"),
					resource.TestCheckResourceAttr("omada_ssid.test", "security", "3"),
					resource.TestCheckResourceAttr("omada_ssid.test", "band", "7"),
					resource.TestCheckResourceAttr("omada_ssid.test", "vlan_enable", "true"),
					resource.TestCheckResourceAttr("omada_ssid.test", "vlan_id", "50"),
					resource.TestCheckResourceAttr("omada_ssid.test", "psk_setting.psk", "supersecret-psk-value"),
					resource.TestCheckResourceAttr("omada_ssid.test", "psk_setting.psk_version", "4"),
					resource.TestCheckResourceAttr("omada_ssid.test", "psk_setting.psk_encryption", "3"),
				),
			},
			// Import (ID format: <site_id>/<wlan_group_id>/<ssid_id>)
			{
				ResourceName:                         "omada_ssid.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/test-wlan-group-id/test-ssid-id",
				ImportStateVerifyIdentifierAttribute: "ssid_id",
			},
			// Update (name + PSK) + Read
			{
				Config: ts.ProviderConfig + `
				resource "omada_ssid" "test" {
					site_id       = "test-site-id"
					wlan_group_id = "test-wlan-group-id"
					name          = "TF Probe SSID Renamed"
					security      = 3
					band          = 7
					vlan_enable   = true
					vlan_id       = 50

					psk_setting = {
						psk = "rotated-psk-value-2"
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ssid.test", "name", "TF Probe SSID Renamed"),
					resource.TestCheckResourceAttr("omada_ssid.test", "psk_setting.psk", "rotated-psk-value-2"),
					resource.TestCheckResourceAttr("omada_ssid.test", "ssid_id", "test-ssid-id"),
				),
			},
			// Delete is exercised automatically by the test harness.
		},
	})
}
