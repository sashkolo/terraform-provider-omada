package dhcpreservation_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// gridResponse wraps one or more reservation rows in the controller's paged grid
// envelope (DhcpReservationOpenApiGridVO): the rows live under result.data with
// result.totalRows describing the full set.
func gridResponse(rows ...map[string]any) string {
	b, _ := json.Marshal(map[string]any{
		"errorCode": 0,
		"msg":       "",
		"result": map[string]any{
			"currentPage": 1,
			"currentSize": len(rows),
			"totalRows":   len(rows),
			"data":        rows,
		},
	})
	return string(b)
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// TestAcc_DhcpReservationResource exercises full CRUD + import for the
// omada_dhcp_reservation resource against an httptest stand-in for the Omada
// Open API. The reservation grid is paginated (result.data + totalRows); the
// modify handler mutates the row in place so the subsequent Read reflects the
// change, exactly like the live controller. The create endpoint returns the new
// id under {id} (ResIdOpenApiVO), but the authoritative read is keyed by MAC.
func TestAcc_DhcpReservationResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// row is the single reservation returned by the grid endpoint. It models the
	// Porch camera camera reserved at 192.168.100.21.
	row := map[string]any{
		"id":          "test-reservation-id",
		"mac":         "00-00-5E-00-53-21",
		"ip":          "192.168.100.21",
		"netId":       "untrusted-net-id",
		"netName":     "Untrusted",
		"description": "Porch camera",
		"status":      true,
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	// Create (POST .../setting/service/dhcp): returns the new id under {id}.
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success.","result":{"id":"test-reservation-id"}}`)
	})

	// Read (GET .../setting/service/dhcp): paged grid with the single row.
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gridResponse(row))
	})

	// Update (PATCH .../setting/service/dhcp/{mac}): reflect ip/description into
	// the grid row so the follow-up Read observes the change.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp/{mac}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ip          string `json:"ip"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Ip != "" {
			row["ip"] = req.Ip
		}
		row["description"] = req.Description
		writeJSON(w, emptyResponse)
	})

	// Delete (DELETE .../setting/service/dhcp/{mac}).
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp/{mac}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: ts.ProviderConfig + `
				resource "omada_dhcp_reservation" "test" {
					site_id     = "test-site-id"
					mac         = "00-00-5E-00-53-21"
					ip          = "192.168.100.21"
					net_id      = "untrusted-net-id"
					description = "Porch camera"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "reservation_id", "test-reservation-id"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "mac", "00-00-5E-00-53-21"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "ip", "192.168.100.21"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "net_id", "untrusted-net-id"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "net_name", "Untrusted"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "description", "Porch camera"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "status", "true"),
				),
			},
			// Import (ID format: <site_id>/<mac>)
			{
				ResourceName:                         "omada_dhcp_reservation.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/00-00-5E-00-53-21",
				ImportStateVerifyIdentifierAttribute: "mac",
			},
			// Update the reserved IP + description in place.
			{
				Config: ts.ProviderConfig + `
				resource "omada_dhcp_reservation" "test" {
					site_id     = "test-site-id"
					mac         = "00-00-5E-00-53-21"
					ip          = "192.168.100.31"
					net_id      = "untrusted-net-id"
					description = "Porch camera (moved)"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "ip", "192.168.100.31"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "description", "Porch camera (moved)"),
				),
			},
		},
	})
}

// TestAcc_DhcpReservationResource_CreateWithoutId covers the create path when
// the controller omits the id in the create response (returns an empty
// envelope): the resource keys its refresh on MAC, so it still recovers full
// state — including reservation_id — from the grid read.
func TestAcc_DhcpReservationResource_CreateWithoutId(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	row := map[string]any{
		"id":      "recovered-reservation-id",
		"mac":     "00-00-5E-00-53-31",
		"ip":      "192.168.110.13",
		"netId":   "personal-net-id",
		"netName": "Personal",
		"status":  true,
	}

	// Create returns success but no result/id, forcing the MAC-based read to
	// supply reservation_id.
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success."}`)
	})
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gridResponse(row))
	})
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp/{mac}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{ "errorCode": 0, "msg": "" }`)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + `
				resource "omada_dhcp_reservation" "test" {
					site_id = "test-site-id"
					mac     = "00-00-5E-00-53-31"
					ip      = "192.168.110.13"
					net_id  = "personal-net-id"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "reservation_id", "recovered-reservation-id"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "ip", "192.168.110.13"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "status", "true"),
				),
			},
		},
	})
}

// TestAcc_DhcpReservationResource_MacSeparatorNormalization asserts the Read
// match is insensitive to MAC case and separator style: the config uses
// dash-uppercase, the grid returns colon-lowercase, and the reservation must
// still be found (no spurious removal / re-create in the follow-up plan).
func TestAcc_DhcpReservationResource_MacSeparatorNormalization(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// Grid returns the MAC colon-separated and lowercased — a different style
	// than the dash-uppercase MAC in the configuration.
	row := map[string]any{
		"id":      "sep-reservation-id",
		"mac":     "00:00:5e:00:53:31",
		"ip":      "192.168.110.13",
		"netId":   "personal-net-id",
		"netName": "Personal",
		"status":  true,
	}

	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success.","result":{"id":"sep-reservation-id"}}`)
	})
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gridResponse(row))
	})
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp/{mac}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{ "errorCode": 0, "msg": "" }`)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + `
				resource "omada_dhcp_reservation" "test" {
					site_id = "test-site-id"
					mac     = "00-00-5E-00-53-31"
					ip      = "192.168.110.13"
					net_id  = "personal-net-id"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "reservation_id", "sep-reservation-id"),
					// mac is preserved as configured (not overwritten by the read).
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "mac", "00-00-5E-00-53-31"),
				),
			},
		},
	})
}

// TestAcc_DhcpReservationResource_NetworkMove is the regression test for
// moving a reservation to another network in place. net_name is computed from
// the network the controller reports; it must be planned as unknown when
// net_id changes, or the apply fails with "inconsistent result after apply"
// because the plan still carried the old network's name.
func TestAcc_DhcpReservationResource_NetworkMove(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	networks := map[string]string{
		"network-a-id": "Network A",
		"network-b-id": "Network B",
	}
	row := map[string]any{
		"id":      "move-reservation-id",
		"mac":     "00-00-5E-00-53-41",
		"ip":      "192.0.2.41",
		"netId":   "network-a-id",
		"netName": "Network A",
		"status":  true,
	}

	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success.","result":{"id":"move-reservation-id"}}`)
	})
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gridResponse(row))
	})
	// The modify handler moves the row to the requested network and reports
	// that network's name, as the controller does.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp/{mac}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ip    string `json:"ip"`
			NetId string `json:"netId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name, ok := networks[req.NetId]
		if !ok {
			http.Error(w, "unknown network", http.StatusBadRequest)
			return
		}
		row["ip"] = req.Ip
		row["netId"] = req.NetId
		row["netName"] = name
		writeJSON(w, `{ "errorCode": 0, "msg": "" }`)
	})
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp/{mac}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{ "errorCode": 0, "msg": "" }`)
	})

	config := func(ip, netID string) string {
		return ts.ProviderConfig + `
		resource "omada_dhcp_reservation" "test" {
			site_id = "test-site-id"
			mac     = "00-00-5E-00-53-41"
			ip      = "` + ip + `"
			net_id  = "` + netID + `"
		}
		`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("192.0.2.41", "network-a-id"),
				Check:  resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "net_name", "Network A"),
			},
			// Move to another network in place: no replacement, and the
			// new network's name is read back.
			{
				Config: config("198.51.100.41", "network-b-id"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("omada_dhcp_reservation.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue("omada_dhcp_reservation.test", tfjsonpath.New("net_name")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "reservation_id", "move-reservation-id"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "net_id", "network-b-id"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "net_name", "Network B"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "ip", "198.51.100.41"),
				),
			},
			// An IP-only change on the same network keeps the known name in
			// the plan (no needless "known after apply").
			{
				Config: config("198.51.100.42", "network-b-id"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue("omada_dhcp_reservation.test", tfjsonpath.New("net_name"), knownvalue.StringExact("Network B")),
					},
				},
				Check: resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "net_name", "Network B"),
			},
		},
	})
}
