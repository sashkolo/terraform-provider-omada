package attackdefensesetting_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_AttackDefenseSettingResource exercises the singleton-blob lifecycle
// plus import against an httptest stand-in. The settings object is mutated in
// place by the modify handler so the next read reflects the change. The PATCH
// handler also asserts the modify body carries the controller's
// specifiedOption.securityEnable key (not the SDK's securityOptionEnable), which
// is the fix that unblocks managed writes on firmware with GET/PATCH symmetry.
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
		"specifiedOptionEnable": true,
		"specifiedOption": map[string]any{
			"noOperationEnable": true,
			"recordRouteEnable": true,
			// Controller key for the IP-security option (SDK misnames it
			// securityOptionEnable); the provider must read and write this key.
			"securityEnable":  true,
			"streamEnable":    true,
			"timestampEnable": true,
		},
		"tcpConnEnable":       true,
		"tcpConnLimit":        int32(300),
		"tcpFinNoAckEnable":   true,
		"tcpScanEnable":       true,
		"tcpScanReject":       true,
		"tcpSrcEnable":        true,
		"tcpSrcLimit":         int32(300),
		"tcpSynFinEnable":     true,
		"udpConnEnable":       true,
		"udpConnLimit":        int32(300),
		"udpSrcEnable":        true,
		"udpSrcLimit":         int32(300),
		"winNukeAttackEnable": true,
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

	// Create/Update (PATCH /attack-defense): assert the body uses the controller's
	// nested key, then merge it so the next read reflects the change.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/attack-defense", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body := string(raw)
		if !strings.Contains(body, `"securityEnable"`) {
			t.Errorf("modify body missing controller key securityEnable: %s", body)
		}
		if strings.Contains(body, `"securityOptionEnable"`) {
			t.Errorf("modify body sent SDK key securityOptionEnable (should be securityEnable): %s", body)
		}

		var patch map[string]any
		if err := json.Unmarshal(raw, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for k, v := range patch {
			settings[k] = v
		}
		writeJSON(w, emptyResponse)
	})

	config := func(pingWan bool) string {
		ping := "false"
		if pingWan {
			ping = "true"
		}
		return ts.ProviderConfig + `
		resource "omada_attack_defense_setting" "test" {
			site_id                 = "test-site-id"
			icmp_conn_enable        = true
			icmp_conn_limit         = 300
			icmp_src_enable         = true
			icmp_src_limit          = 300
			large_ping_enable       = true
			large_ping_threshold    = 1024
			ping_death_enable       = true
			ping_wan_enable         = ` + ping + `
			specified_option_enable = true
			specified_option = {
				no_operation_enable    = true
				record_route_enable    = true
				security_option_enable = true
				stream_enable          = true
				timestamp_enable       = true
			}
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
		`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: config(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "ping_wan_enable", "false"),
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "tcp_scan_reject", "true"),
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "icmp_conn_limit", "300"),
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "specified_option.security_option_enable", "true"),
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
				Config: config(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_attack_defense_setting.test", "ping_wan_enable", "true"),
				),
			},
			// Delete is a no-op (exercised automatically by the harness).
		},
	})
}
