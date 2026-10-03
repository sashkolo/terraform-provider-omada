package aclconfigmode_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// guardedConfig reads the mode with the postcondition a configuration uses to
// fail the plan when someone switches the mode in the UI.
const guardedConfig = `
data "omada_gateway_acl_config_mode" "test" {
	site_id = "test-site-id"

	lifecycle {
		postcondition {
			condition     = self.mode == 0
			error_message = "The gateway ACL mode is no longer profile-based."
		}
	}
}
`

func TestAcc_GatewayAclConfigMode(t *testing.T) {
	var mu sync.Mutex
	mode := 0
	fail := false
	ts := acctest.NewTestServer(t)
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-config-mode", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if fail {
			_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"mode": mode}})
	})
	setMode := func(m int, f bool) func() {
		return func() {
			mu.Lock()
			defer mu.Unlock()
			mode, fail = m, f
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + guardedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.omada_gateway_acl_config_mode.test", "mode", "0"),
					resource.TestCheckResourceAttr("data.omada_gateway_acl_config_mode.test", "custom", "false"),
				),
			},
			{
				// Switched to custom ACLs in the UI: the plan fails.
				PreConfig:   setMode(1, false),
				Config:      ts.ProviderConfig + guardedConfig,
				ExpectError: regexp.MustCompile(`no longer profile-based`),
			},
			{
				// A controller error fails the read rather than reporting a mode.
				PreConfig:   setMode(0, true),
				Config:      ts.ProviderConfig + guardedConfig,
				ExpectError: regexp.MustCompile(`(?s)reading gateway ACL config mode.*-1001`),
			},
		},
	})
}
