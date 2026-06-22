package lannetwork_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAcc_LanNetworkResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// readResponse is mutated by the modify handler so the subsequent GET
	// reflects the in-place update, exactly the way the live controller does.
	readResponse := `{
		"errorCode": 0,
		"msg": "",
		"result": {
			"id": "test-network-id",
			"name": "TF Probe VLAN",
			"vlan": 999,
			"vlanType": 0,
			"gatewaySubnet": "192.168.199.1/24",
			"domain": "probe.local",
			"igmpSnoopEnable": false,
			"deviceType": 1,
			"purpose": 0,
			"dhcpServer": {
				"gateway": "192.168.199.1",
				"ip": "192.168.199.1",
				"netmask": "255.255.255.0",
				"leasetime": 1440,
				"priDns": "192.168.199.1",
				"sndDns": "8.8.8.8",
				"ipRangePool": [{ "startIp": "192.168.199.100", "endIp": "192.168.199.200" }]
			}
		}
	}`

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Create (Confirm create lan network)
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/networks/confirm", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{
			"errorCode": 0,
			"msg": "",
			"result": { "networkIdList": ["test-network-id"] }
		}`)
	})

	// Read (Get LAN network)
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/lan-networks/{networkId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, readResponse)
	})

	// Update (Confirm modify lan network): decode the body and reflect name +
	// lease-time changes into the read response.
	mux.HandleFunc("PUT /openapi/v1/{omadacId}/sites/{siteId}/networks/{networkId}/confirm", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			LanNetwork struct {
				Name       string `json:"name"`
				DhcpServer *struct {
					Leasetime int32 `json:"leasetime"`
				} `json:"dhcpServer,omitempty"`
			} `json:"lanNetwork"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		leaseTime := 1440
		if req.LanNetwork.DhcpServer != nil {
			leaseTime = int(req.LanNetwork.DhcpServer.Leasetime)
		}

		readResponse = fmt.Sprintf(`{
			"errorCode": 0,
			"msg": "",
			"result": {
				"id": "test-network-id",
				"name": %q,
				"vlan": 999,
				"vlanType": 0,
				"gatewaySubnet": "192.168.199.1/24",
				"domain": "probe.local",
				"igmpSnoopEnable": false,
				"deviceType": 1,
				"purpose": 0,
				"dhcpServer": {
					"gateway": "192.168.199.1",
					"ip": "192.168.199.1",
					"netmask": "255.255.255.0",
					"leasetime": %d,
					"priDns": "192.168.199.1",
					"sndDns": "8.8.8.8",
					"ipRangePool": [{ "startIp": "192.168.199.100", "endIp": "192.168.199.200" }]
				}
			}
		}`, req.LanNetwork.Name, leaseTime)

		writeJSON(w, emptyResponse)
	})

	// Delete
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/lan-networks/{networkId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read
			{
				Config: ts.ProviderConfig + `
				resource "omada_lan_network" "test" {
					site_id        = "test-site-id"
					name           = "TF Probe VLAN"
					vlan_id        = 999
					gateway_subnet = "192.168.199.1/24"
					domain         = "probe.local"

					dhcp_server = {
						gateway   = "192.168.199.1"
						ip        = "192.168.199.1"
						netmask   = "255.255.255.0"
						leasetime = 1440
						pri_dns   = "192.168.199.1"
						snd_dns   = "8.8.8.8"
						ip_range_pool = [
							{ start = "192.168.199.100", end = "192.168.199.200" }
						]
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "network_id", "test-network-id"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "name", "TF Probe VLAN"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "vlan_id", "999"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "gateway_subnet", "192.168.199.1/24"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "domain", "probe.local"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "device_type", "1"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_server.leasetime", "1440"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_server.ip_range_pool.0.start", "192.168.199.100"),
				),
			},
			// Import (ID format: <site_id>/<network_id>)
			{
				ResourceName:                         "omada_lan_network.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/test-network-id",
				ImportStateVerifyIdentifierAttribute: "network_id",
			},
			// Update (name + lease time) + Read
			{
				Config: ts.ProviderConfig + `
				resource "omada_lan_network" "test" {
					site_id        = "test-site-id"
					name           = "TF Probe VLAN Renamed"
					vlan_id        = 999
					gateway_subnet = "192.168.199.1/24"
					domain         = "probe.local"

					dhcp_server = {
						gateway   = "192.168.199.1"
						ip        = "192.168.199.1"
						netmask   = "255.255.255.0"
						leasetime = 720
						pri_dns   = "192.168.199.1"
						snd_dns   = "8.8.8.8"
						ip_range_pool = [
							{ start = "192.168.199.100", end = "192.168.199.200" }
						]
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "name", "TF Probe VLAN Renamed"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_server.leasetime", "720"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "vlan_id", "999"),
				),
			},
			// Delete is exercised automatically by the test harness.
		},
	})
}
