package lannetwork_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAcc_LanNetworkResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// listRow is the single row returned by the paged list endpoint for the
	// test network. It is mutated by the modify handler so the subsequent Read
	// reflects the in-place update, exactly the way the live controller does.
	listRow := map[string]any{
		"id":              "test-network-id",
		"name":            "TF Probe VLAN",
		"vlan":            999,
		"vlanType":        0,
		"purpose":         1,
		"gatewaySubnet":   "192.168.199.1/24",
		"domain":          "probe.local",
		"igmpSnoopEnable": false,
		"interfaceIds":    []string{"4_a4a0ba6187b44f0189b28f976417aadc"},
		"dhcpSettingsVO": map[string]any{
			"enable":      true,
			"dhcpns":      "manual",
			"gateway":     "192.168.199.1",
			"ipaddrStart": "192.168.199.100",
			"ipaddrEnd":   "192.168.199.200",
			"leasetime":   1440,
			"priDns":      "192.168.199.1",
			"sndDns":      "8.8.8.8",
		},
	}

	listResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "",
			"result": map[string]any{
				"totalRows":   1,
				"currentPage": 1,
				"currentSize": 1000,
				"data":        []any{listRow},
			},
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Create (POST /lan-networks): the controller wraps the new id in the
	// standard envelope; the resource falls back to a name lookup when the id
	// cannot be decoded from the deprecated response shape.
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/lan-networks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"","result":{"id":"test-network-id"}}`)
	})

	// Read (GET /lan-networks, paged list).
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/lan-networks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, listResponse())
	})

	// Update (PATCH /lan-networks/{networkId}): decode the body and reflect name
	// + lease-time changes into the list row.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/lan-networks/{networkId}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name           string `json:"name"`
			DhcpSettingsVO *struct {
				Leasetime int32 `json:"leasetime"`
			} `json:"dhcpSettingsVO,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			listRow["name"] = req.Name
		}
		if req.DhcpSettingsVO != nil {
			if dhcp, ok := listRow["dhcpSettingsVO"].(map[string]any); ok {
				dhcp["leasetime"] = req.DhcpSettingsVO.Leasetime
			}
		}

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
					interface_ids  = ["4_a4a0ba6187b44f0189b28f976417aadc"]

					dhcp_settings = {
						enable      = true
						dhcpns      = "manual"
						gateway     = "192.168.199.1"
						ipaddr_start = "192.168.199.100"
						ipaddr_end   = "192.168.199.200"
						leasetime   = 1440
						pri_dns     = "192.168.199.1"
						snd_dns     = "8.8.8.8"
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "network_id", "test-network-id"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "name", "TF Probe VLAN"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "vlan_id", "999"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "purpose", "1"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "gateway_subnet", "192.168.199.1/24"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "domain", "probe.local"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "interface_ids.#", "1"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "interface_ids.0", "4_a4a0ba6187b44f0189b28f976417aadc"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.leasetime", "1440"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.ipaddr_start", "192.168.199.100"),
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
					interface_ids  = ["4_a4a0ba6187b44f0189b28f976417aadc"]

					dhcp_settings = {
						enable      = true
						dhcpns      = "manual"
						gateway     = "192.168.199.1"
						ipaddr_start = "192.168.199.100"
						ipaddr_end   = "192.168.199.200"
						leasetime   = 720
						pri_dns     = "192.168.199.1"
						snd_dns     = "8.8.8.8"
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "name", "TF Probe VLAN Renamed"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.leasetime", "720"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "vlan_id", "999"),
				),
			},
			// Delete is exercised automatically by the test harness.
		},
	})
}
