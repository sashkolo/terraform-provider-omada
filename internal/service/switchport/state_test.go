package switchport_test

import (
	"encoding/json"
	"net/http"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// fakeSwitchController models the parts of the controller that state accuracy
// depends on (homelab #514): a switch overview that answers -39050 ("This
// device does not exist") once the switch is removed from the controller.
type fakeSwitchController struct {
	mu      sync.Mutex
	removed bool
}

func newFakeSwitchController(t *testing.T) (*fakeSwitchController, *acctest.TestServer) {
	f := &fakeSwitchController{}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	ts.Mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/switches/{switchMac}/ports/{port}", func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/switches/{switchMac}", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.removed {
			write(w, map[string]any{"errorCode": -39050, "msg": "This device does not exist."})
			return
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"mac": "E4-FA-C4-9E-CD-87",
			"portList": []any{map[string]any{
				"port": 8, "name": "Port8", "profileId": "test-profile-id", "profileName": "Outdoor-Untrusted",
				"profileOverrideEnable": true, "poeMode": 1, "status": 1, "lagPort": false,
			}},
		}})
	})
	return f, ts
}

const stateTestSwitchPort = `
resource "omada_switch_port" "test" {
	site_id    = "test-site-id"
	switch_mac = "E4-FA-C4-9E-CD-87"
	port       = 8
	profile_id = "test-profile-id"
}
`

// A switch removed from the controller must drop its ports from state, not
// fail every refresh. On 0.14.0 Read turned -39050 into an error, so no plan
// or destroy could run until the resource was removed from state by hand.
func TestAcc_SwitchPortSwitchRemoved(t *testing.T) {
	f, ts := newFakeSwitchController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestSwitchPort},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					f.removed = true
				},
				Config:             ts.ProviderConfig + stateTestSwitchPort,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
