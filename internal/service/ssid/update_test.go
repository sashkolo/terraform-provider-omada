package ssid_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// An edit must not reset settings the config leaves unset (homelab #515). On
// 0.14.0 the unset Optional+Computed attributes were unknown on update and the
// provider sent hard-coded defaults, so renaming an SSID (or rotating its PSK)
// switched 802.11r, MLO and group-key rekey off; autoWanAccess, which the
// resource doesn't model, was left to the endpoint.
func TestAcc_SsidUpdateKeepsUnsetLiveSettings(t *testing.T) {
	f, ts := newFakeSsidController(t)

	renamed := ts.ProviderConfig + `
resource "omada_ssid" "test" {
	site_id       = "test-site-id"
	wlan_group_id = "test-wlan-group-id"
	name          = "TF State SSID renamed"
	security      = 3
	band          = 3

	psk_setting = {
		psk = "state-test-psk-value"
	}
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestSsid},
			{
				// Someone turns these on in the controller UI.
				PreConfig: f.set(func(f *fakeSsidController) {
					for _, row := range f.rows {
						row["enable11r"] = true
						row["mloEnable"] = true
						row["autoWanAccess"] = true
						psk := row["pskSetting"].(map[string]any)
						psk["gikRekeyPskEnable"] = true
					}
				}),
				Config: renamed,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ssid.test", "enable_11r", "true"),
					resource.TestCheckResourceAttr("omada_ssid.test", "mlo_enable", "true"),
					resource.TestCheckResourceAttr("omada_ssid.test", "psk_setting.gik_rekey_psk_enable", "true"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						body := f.lastPatch
						psk, _ := body["pskSetting"].(map[string]any)
						for key, got := range map[string]any{
							"enable11r":                    body["enable11r"],
							"mloEnable":                    body["mloEnable"],
							"autoWanAccess":                body["autoWanAccess"],
							"pskSetting.gikRekeyPskEnable": psk["gikRekeyPskEnable"],
						} {
							if got != true {
								return fmt.Errorf("update sent %s=%v, want the live true", key, got)
							}
						}
						return nil
					},
				),
			},
		},
	})
}
