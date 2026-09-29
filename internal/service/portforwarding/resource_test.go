package portforwarding_test

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAcc_PortForwardingResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	row := map[string]any{
		"id":                 "test-port-forwarding-id",
		"dMZ":                false,
		"name":               "WireGuard",
		"status":             true,
		"externalPort":       "51820",
		"forwardIp":          "192.168.1.5",
		"forwardPort":        "51820",
		"protocol":           int32(2),
		"from":               int32(0),
		"interfaceWanPortId": []any{"wan-primary"},
		"limitedAddresses":   []any{},
		"virtualWanId":       []any{},
		"wanIps":             []any{},
	}

	listResponse := func(rows ...map[string]any) string {
		data := make([]any, 0, len(rows))
		for _, item := range rows {
			data = append(data, item)
		}
		body, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "Success.",
			"result": map[string]any{
				"currentPage": 1,
				"currentSize": len(rows),
				"totalRows":   len(rows),
				"data":        data,
			},
		})
		return string(body)
	}
	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	var listReads int32
	var rowMutex sync.Mutex
	var pendingRow map[string]any
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/nat/port-forwardings", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if dmz, ok := body["dMZ"].(bool); !ok || dmz {
			http.Error(w, "DMZ must be false", http.StatusBadRequest)
			return
		}
		// The live endpoint commonly returns success without an id, exercising
		// the resource's eventually-consistent name lookup.
		writeJSON(w, `{"errorCode":0,"msg":"Success."}`)
	})
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/nat/port-forwardings", func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&listReads, 1) == 1 {
			writeJSON(w, listResponse())
			return
		}
		rowMutex.Lock()
		defer rowMutex.Unlock()
		if pendingRow != nil {
			// The first read after an update still exposes the old row, then the
			// controller's list catches up. Update must retry until it observes
			// the desired values instead of flattening this stale response.
			writeJSON(w, listResponse(row))
			row = pendingRow
			pendingRow = nil
			return
		}
		writeJSON(w, listResponse(row))
	})
	mux.HandleFunc("PUT /openapi/v1/{omadacId}/sites/{siteId}/nat/port-forwardings/{portForwardingId}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name               string              `json:"name"`
			Status             bool                `json:"status"`
			ExternalPort       string              `json:"externalPort"`
			ForwardIp          string              `json:"forwardIp"`
			ForwardPort        string              `json:"forwardPort"`
			Protocol           int32               `json:"protocol"`
			From               int32               `json:"from"`
			InterfaceWanPortId []string            `json:"interfaceWanPortId"`
			LimitedAddresses   []string            `json:"limitedAddresses"`
			VirtualWanId       []string            `json:"virtualWanId"`
			WanIps             []map[string]string `json:"wanIps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rowMutex.Lock()
		pendingRow = make(map[string]any, len(row))
		for key, value := range row {
			pendingRow[key] = value
		}
		pendingRow["name"] = body.Name
		pendingRow["status"] = body.Status
		pendingRow["externalPort"] = body.ExternalPort
		pendingRow["forwardIp"] = body.ForwardIp
		pendingRow["forwardPort"] = body.ForwardPort
		pendingRow["protocol"] = body.Protocol
		pendingRow["from"] = body.From
		pendingRow["interfaceWanPortId"] = body.InterfaceWanPortId
		pendingRow["limitedAddresses"] = body.LimitedAddresses
		pendingRow["virtualWanId"] = body.VirtualWanId
		pendingRow["wanIps"] = body.WanIps
		rowMutex.Unlock()
		writeJSON(w, `{"errorCode":0,"msg":"Success."}`)
	})
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/nat/port-forwardings/{portForwardingId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success."}`)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + `
resource "omada_port_forwarding" "wireguard" {
  site_id         = "test-site-id"
  name            = "WireGuard"
  status          = true
  external_port   = "51820"
  forward_ip      = "192.168.1.5"
  forward_port    = "51820"
  protocol        = 2
  wan_port_ids    = ["wan-primary"]
  source_addresses = []
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "port_forwarding_id", "test-port-forwarding-id"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "name", "WireGuard"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "external_port", "51820"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "forward_ip", "192.168.1.5"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "forward_port", "51820"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "protocol", "2"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "wan_port_ids.0", "wan-primary"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "source_addresses.#", "0"),
				),
			},
			{
				ResourceName:                         "omada_port_forwarding.wireguard",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/test-port-forwarding-id",
				ImportStateVerifyIdentifierAttribute: "port_forwarding_id",
			},
			{
				Config: ts.ProviderConfig + `
resource "omada_port_forwarding" "wireguard" {
  site_id         = "test-site-id"
  name            = "WireGuard restricted"
  status          = true
  external_port   = "51820"
  forward_ip      = "192.168.1.5"
  forward_port    = "51820"
  protocol        = 2
  wan_port_ids    = ["wan-primary"]
  source_addresses = ["203.0.113.10"]
  wan_ips = {
    "wan-primary" = "198.51.100.20"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "name", "WireGuard restricted"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "source_addresses.0", "203.0.113.10"),
					resource.TestCheckResourceAttr("omada_port_forwarding.wireguard", "wan_ips.wan-primary", "198.51.100.20"),
				),
			},
		},
	})
}
