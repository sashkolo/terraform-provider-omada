package switchport_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_SwitchPortResource exercises Create(modify)/Read/Update/Import for the
// omada_switch_port resource against an httptest stand-in for the Omada Open
// API. A physical port always exists, so there is no create/delete endpoint:
// Create and Update both PATCH the per-port config, and Read selects the port
// from the switch-overview portList. The port row is mutated in place by the
// modify handler so the subsequent Read reflects the change.
func TestAcc_SwitchPortResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	// portRow is the single port entry returned inside the switch-overview
	// portList. It models Main Switch port 8 assigned to Outdoor-Untrusted.
	portRow := map[string]any{
		"port":                  int32(8),
		"name":                  "Port8",
		"profileId":             "test-profile-id",
		"profileName":           "Outdoor-Untrusted",
		"profileOverrideEnable": true,
		"poeMode":               int32(1),
		"status":                int32(1),
		"lagPort":               false,
	}

	switchResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "",
			"result": map[string]any{
				"mac":      "E4-FA-C4-9E-CD-87",
				"portList": []any{portRow},
			},
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "Success." }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	// Modify (PATCH .../switches/{mac}/ports/{port}): decode the body and reflect
	// the profile assignment change into the port row.
	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/switches/{switchMac}/ports/{port}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ProfileId *string `json:"profileId"`
			Name      *string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ProfileId != nil && *req.ProfileId != "" {
			portRow["profileId"] = *req.ProfileId
		}
		if req.Name != nil && *req.Name != "" {
			portRow["name"] = *req.Name
		}
		writeJSON(w, emptyResponse)
	})

	// Read (GET .../switches/{mac}): switch overview with portList.
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/switches/{switchMac}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, switchResponse())
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create(modify) + Read.
			{
				Config: ts.ProviderConfig + `
				resource "omada_switch_port" "test" {
					site_id    = "test-site-id"
					switch_mac = "E4-FA-C4-9E-CD-87"
					port       = 8
					profile_id = "test-profile-id"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_switch_port.test", "site_id", "test-site-id"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "switch_mac", "E4-FA-C4-9E-CD-87"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "port", "8"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "profile_id", "test-profile-id"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "profile_name", "Outdoor-Untrusted"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "profile_override_enable", "true"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "poe", "1"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "disabled", "false"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "lag_port", "false"),
					resource.TestCheckResourceAttr("omada_switch_port.test", "name", "Port8"),
				),
			},
			// Import (ID format: <site_id>/<switch_mac>/<port>)
			{
				ResourceName:                         "omada_switch_port.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/E4-FA-C4-9E-CD-87/8",
				ImportStateVerifyIdentifierAttribute: "profile_id",
			},
			// Update the profile assignment in place.
			{
				Config: ts.ProviderConfig + `
				resource "omada_switch_port" "test" {
					site_id    = "test-site-id"
					switch_mac = "E4-FA-C4-9E-CD-87"
					port       = 8
					profile_id = "test-profile-id-2"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_switch_port.test", "profile_id", "test-profile-id-2"),
				),
			},
		},
	})
}
