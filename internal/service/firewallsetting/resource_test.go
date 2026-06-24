package firewallsetting_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_FirewallSettingResource exercises the singleton-blob lifecycle
// (create/modify = apply the whole object; read = GET; delete = no-op) plus
// import against an httptest stand-in. The settings row is mutated in place by
// the modify handler so the subsequent Read reflects the change.
func TestAcc_FirewallSettingResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// settings is the object returned by GET /firewall. The modify handler
	// mutates it so the next read reflects the in-place update.
	settings := map[string]any{
		"broadcastPing":    false,
		"icmp":             int32(30),
		"other":            int32(600),
		"receiveRedirects": false,
		"sendRedirects":    true,
		"synCookies":       true,
		"tcpClose":         int32(10),
		"tcpCloseWait":     int32(60),
		"tcpEstablished":   int32(7440),
		"tcpFinWait":       int32(120),
		"tcpLastAck":       int32(30),
		"tcpSynReceive":    int32(60),
		"tcpSynSent":       int32(120),
		"tcpTimeWait":      int32(120),
		"udpOther":         int32(60),
		"udpStream":        int32(180),
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

	// Read (GET /firewall).
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/firewall", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, settingsResponse())
	})

	// Create/Update (PATCH /firewall): decode the body and reflect the ICMP timeout
	// change into the in-memory settings.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/firewall", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Icmp int32 `json:"icmp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		settings["icmp"] = req.Icmp
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: ts.ProviderConfig + `
				resource "omada_firewall_setting" "test" {
					site_id           = "test-site-id"
					broadcast_ping    = false
					icmp              = 30
					other             = 600
					receive_redirects = false
					send_redirects    = true
					syn_cookies       = true
					tcp_close         = 10
					tcp_close_wait    = 60
					tcp_established   = 7440
					tcp_fin_wait      = 120
					tcp_last_ack      = 30
					tcp_syn_receive   = 60
					tcp_syn_sent      = 120
					tcp_time_wait     = 120
					udp_other         = 60
					udp_stream        = 180
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_firewall_setting.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_firewall_setting.test", "icmp", "30"),
					resource.TestCheckResourceAttr("omada_firewall_setting.test", "tcp_established", "7440"),
					resource.TestCheckResourceAttr("omada_firewall_setting.test", "syn_cookies", "true"),
					resource.TestCheckResourceAttr("omada_firewall_setting.test", "broadcast_ping", "false"),
				),
			},
			// Import (ID format: <site_id>).
			{
				ResourceName:                         "omada_firewall_setting.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id",
				ImportStateVerifyIdentifierAttribute: "site_id",
			},
			// Update (icmp timeout) + Read.
			{
				Config: ts.ProviderConfig + `
				resource "omada_firewall_setting" "test" {
					site_id           = "test-site-id"
					broadcast_ping    = false
					icmp              = 60
					other             = 600
					receive_redirects = false
					send_redirects    = true
					syn_cookies       = true
					tcp_close         = 10
					tcp_close_wait    = 60
					tcp_established   = 7440
					tcp_fin_wait      = 120
					tcp_last_ack      = 30
					tcp_syn_receive   = 60
					tcp_syn_sent      = 120
					tcp_time_wait     = 120
					udp_other         = 60
					udp_stream        = 180
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_firewall_setting.test", "icmp", "60"),
				),
			},
			// Delete is a no-op (exercised automatically by the harness).
		},
	})
}
