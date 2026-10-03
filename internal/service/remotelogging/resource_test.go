package remotelogging_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeRemoteLogging stands in for the controller's remote-logging singleton.
type fakeRemoteLogging struct {
	mu        sync.Mutex
	setting   map[string]any // the remoteLog object
	patches   int
	lastPatch map[string]any // the remoteLog object of the most recent PATCH
	failPatch bool           // answer PATCH with a controller error
}

func newFakeRemoteLogging(t *testing.T) (*fakeRemoteLogging, *acctest.TestServer) {
	f := &fakeRemoteLogging{setting: map[string]any{
		"enable": true, "host": "192.0.2.10", "port": 514, "moreClientLog": true, "resource": 0,
	}}
	ts := acctest.NewTestServer(t)
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/remote-logging", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"remoteLog": f.setting}})
	})
	ts.Mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/remote-logging", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RemoteLog map[string]any `json:"remoteLog"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failPatch {
			write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		f.patches++
		f.lastPatch = body.RemoteLog
		// PATCH semantics: fields the body leaves out keep their value.
		for k, v := range body.RemoteLog {
			f.setting[k] = v
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeRemoteLogging) set(fn func(f *fakeRemoteLogging)) func() {
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		fn(f)
	}
}

// remoteLoggingConfig renders the setting pointing at 192.0.2.10 on a given
// port, leaving more_client_log unset.
func remoteLoggingConfig(port int) string {
	return fmt.Sprintf(`
resource "omada_remote_logging_setting" "test" {
	site_id = "test-site-id"
	enable  = true
	host    = "192.0.2.10"
	port    = %d
}
`, port)
}

// Adopting the live setting without more_client_log in the config keeps the
// live value: the PATCH leaves it out, and state reads the live true.
func TestAcc_RemoteLoggingKeepsUnsetClientLog(t *testing.T) {
	f, ts := newFakeRemoteLogging(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + remoteLoggingConfig(514),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_remote_logging_setting.test", "more_client_log", "true"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if _, sent := f.lastPatch["moreClientLog"]; sent {
							return fmt.Errorf("create sent moreClientLog although the config leaves it unset")
						}
						return nil
					},
				),
			},
			{
				ResourceName:                         "omada_remote_logging_setting.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id",
				ImportStateVerifyIdentifierAttribute: "site_id",
			},
			{
				// An edit of another field sends the kept value, never a default.
				Config: ts.ProviderConfig + remoteLoggingConfig(1514),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_remote_logging_setting.test", "port", "1514"),
					resource.TestCheckResourceAttr("omada_remote_logging_setting.test", "more_client_log", "true"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.lastPatch["moreClientLog"] != true {
							return fmt.Errorf("update sent moreClientLog=%v, want the live true", f.lastPatch["moreClientLog"])
						}
						return nil
					},
				),
			},
		},
	})
}

// A host or enable change made in the controller UI shows up as drift, and the
// apply restores the configured target.
func TestAcc_RemoteLoggingDrift(t *testing.T) {
	f, ts := newFakeRemoteLogging(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + remoteLoggingConfig(514)},
			{
				PreConfig: f.set(func(f *fakeRemoteLogging) {
					f.setting["host"] = "192.0.2.99"
					f.setting["enable"] = false
				}),
				Config:             ts.ProviderConfig + remoteLoggingConfig(514),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: ts.ProviderConfig + remoteLoggingConfig(514),
				Check: func(*terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if f.setting["host"] != "192.0.2.10" || f.setting["enable"] != true {
						return fmt.Errorf("apply left host=%v enable=%v, want the configured target", f.setting["host"], f.setting["enable"])
					}
					return nil
				},
			},
		},
	})
}

// Destroy leaves the live setting alone: it is a singleton, and resetting it
// would silently stop the site's logs.
func TestAcc_RemoteLoggingDestroyIsNoOp(t *testing.T) {
	f, ts := newFakeRemoteLogging(t)
	var patchesBefore int

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + remoteLoggingConfig(514),
				Check: func(*terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					patchesBefore = f.patches
					return nil
				},
			},
		},
		CheckDestroy: func(*terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.patches != patchesBefore || f.setting["enable"] != true {
				return fmt.Errorf("destroy changed the live setting (patches %d -> %d, enable=%v)", patchesBefore, f.patches, f.setting["enable"])
			}
			return nil
		},
	})
}

// A port outside 1-65535 fails validation; a controller error on write fails
// the apply instead of being recorded as success.
func TestAcc_RemoteLoggingErrors(t *testing.T) {
	f, ts := newFakeRemoteLogging(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + remoteLoggingConfig(70000),
				ExpectError: regexp.MustCompile(`port must be within 1-65535`),
			},
			{
				PreConfig:   f.set(func(f *fakeRemoteLogging) { f.failPatch = true }),
				Config:      ts.ProviderConfig + remoteLoggingConfig(514),
				ExpectError: regexp.MustCompile(`(?s)creating remote logging setting.*-1001`),
			},
		},
	})
}
