package attackdefensesetting_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_AttackDefenseSettingResource exercises the singleton-blob lifecycle
// plus import against an httptest stand-in. The settings object is mutated in
// place by the modify handler so the next read reflects the change.
func TestAcc_AttackDefenseSettingResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	settings := map[string]any{
		"icmpConnEnable":        true,
		"icmpConnLimit":         int32(300),
		"icmpSrcEnable":         true,
		"icmpSrcLimit":          int32(300),
		"largePingEnable":       true,
		"largePingThreshold":    int32(1024),
		"pingDeathEnable":       true,
		"pingWanEnable":         false,
		"specifiedOptionEnable": false,
		"tcpConnEnable":         true,
		"tcpConnLimit":          int32(300),
		"tcpFinNoAckEnable":     true,
		"tcpScanEnable":         true,
		"tcpScanReject":         true,
		"tcpSrcEnable":          true,
		"tcpSrcLimit":           int32(300),
		"tcpSynFinEnable":       true,
		"udpConnEnable":         true,
		"udpConnLimit":          int32(300),
		"udpSrcEnable":          true,
		"udpSrcLimit":           int32(300),
		"winNukeAttackEnable":   true,
	}

	settingsResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "Success.",
			"result":    settings,
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Read (GET /attack-defense).
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/attack-defense", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, settingsResponse())
	})

	// Create/Update (PATCH /attack-defense): reflect the pingWanEnable change.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/attack-defense", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			PingWanEnable bool `json:"pingWanEnable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		settings["pingWanEnable"] = req.PingWanEnable
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: ts.ProviderConfig + `
				resource "omada_attack_defense_setting" "test" {
					site_id                = "test-site-id"
					icmp_conn_enable       = true
					icmp_conn_limit        = 300
					icmp_src_enable        = true
					icmp_src_limit         = 300
					large_ping_enable      = true
					large_ping_threshold   = 1024
					ping_death_enable      = true
					ping_wan_enable        = false
					specified_option_enable = false
					tcp_conn_enable        = true
					tcp_conn_limit         = 300
					tcp_fin_no_ack_enable  = true
					tcp_scan_enable        = true
					tcp_scan_reject        = true
					tcp_src_enable         = true
					tcp_src_limit          = 300
					tcp_syn_fin_enable     = true
					udp_conn_enable        = true
					udp_conn_limit         = 300
					udp_src_enable         = true
					udp_src_limit          = 300
					win_nuke_attack_enable = true
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "ping_wan_enable", "false"),
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "tcp_scan_reject", "true"),
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "icmp_conn_limit", "300"),
				),
			},
			// Import (ID format: <site_id>).
			{
				ResourceName:                         "omada_attack_defense_setting.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id",
				ImportStateVerifyIdentifierAttribute: "site_id",
			},
			// Update (ping_wan_enable) + Read.
			{
				Config: ts.ProviderConfig + `
				resource "omada_attack_defense_setting" "test" {
					site_id                = "test-site-id"
					icmp_conn_enable       = true
					icmp_conn_limit        = 300
					icmp_src_enable        = true
					icmp_src_limit         = 300
					large_ping_enable      = true
					large_ping_threshold   = 1024
					ping_death_enable      = true
					ping_wan_enable        = true
					specified_option_enable = false
					tcp_conn_enable        = true
					tcp_conn_limit         = 300
					tcp_fin_no_ack_enable  = true
					tcp_scan_enable        = true
					tcp_scan_reject        = true
					tcp_src_enable         = true
					tcp_src_limit          = 300
					tcp_syn_fin_enable     = true
					udp_conn_enable        = true
					udp_conn_limit         = 300
					udp_src_enable         = true
					udp_src_limit          = 300
					win_nuke_attack_enable = true
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "ping_wan_enable", "true"),
				),
			},
			// Delete is a no-op (exercised automatically by the harness).
		},
	})
}
