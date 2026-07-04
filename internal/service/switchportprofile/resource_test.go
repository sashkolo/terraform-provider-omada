package switchportprofile_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_SwitchPortProfileResource exercises full CRUD + import for the
// omada_switch_port_profile resource against an httptest stand-in for the Omada
// Open API. The list row is mutated in place by the modify handler so the
// subsequent Read reflects the change, exactly like the live controller. The
// v1 create endpoint returns the new id under {id}; the resource takes it
// straight from the create response.
func TestAcc_SwitchPortProfileResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// listRow is the single row returned by the paged LAN-profile list endpoint.
	// It models an access/untagged Outdoor-Untrusted profile: a native VLAN and
	// zero tagged networks.
	listRow := map[string]any{
		"id":                   "test-profile-id",
		"name":                 "Outdoor-Untrusted",
		"nativeNetworkId":      "net-outdoor",
		"tagNetworkIds":        []any{},
		"untagNetworkIds":      []any{},
		"poe":                  int32(2),
		"dot1x":                int32(2),
		"bandWidthCtrlType":    int32(0),
		"portIsolationEnable":  false,
		"lldpMedEnable":        true,
		"loopbackDetectEnable": false,
		"spanningTreeEnable":   false,
		"flag":                 int32(2),
		"type":                 int32(2),
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

	// Create (POST .../lan-profiles): the v1 create endpoint returns the new id
	// under {id} (ResponseIdVO).
	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/lan-profiles", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success.","result":{"id":"test-profile-id"}}`)
	})

	// Read (GET .../lan-profiles, paged list).
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/lan-profiles", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, listResponse())
	})

	// Update (PATCH .../lan-profiles/{profileId}): decode the body and reflect
	// the name change into the list row.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/lan-profiles/{profileId}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name != "" {
			listRow["name"] = req.Name
		}
		writeJSON(w, emptyResponse)
	})

	// Delete (DELETE .../lan-profiles/{profileId}).
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/lan-profiles/{profileId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create + Read. tagged/untagged network lists are declared empty
			// (an access port); the toggles are computed from the controller.
			{
				Config: ts.ProviderConfig + `
				resource "omada_switch_port_profile" "test" {
					site_id            = "test-site-id"
					name               = "Outdoor-Untrusted"
					native_network_id  = "net-outdoor"
					tagged_network_ids = []
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "profile_id", "test-profile-id"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "name", "Outdoor-Untrusted"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "native_network_id", "net-outdoor"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "tagged_network_ids.#", "0"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "untagged_network_ids.#", "0"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "poe", "2"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "dot1x", "2"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "port_isolation_enable", "false"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "flag", "2"),
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "type", "2"),
				),
			},
			// Import (ID format: <site_id>/<profile_id>)
			{
				ResourceName:                         "omada_switch_port_profile.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/test-profile-id",
				ImportStateVerifyIdentifierAttribute: "profile_id",
			},
			// Update the name in place.
			{
				Config: ts.ProviderConfig + `
				resource "omada_switch_port_profile" "test" {
					site_id            = "test-site-id"
					name               = "Outdoor-Untrusted-2"
					native_network_id  = "net-outdoor"
					tagged_network_ids = []
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_switch_port_profile.test", "name", "Outdoor-Untrusted-2"),
				),
			},
		},
	})
}
