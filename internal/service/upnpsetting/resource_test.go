package upnpsetting_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const resourceName = "omada_upnp_setting.test"

// fakeUpnp stands in for the controller's UPnP singleton. PUT replaces the
// whole setting, as the endpoint does, so a write that leaves a field out
// clears it here and a partial update shows up as lost state.
type fakeUpnp struct {
	mu            sync.Mutex
	enable        bool
	networkIds    []string
	wanPortIds    []string
	supportDsLite bool           // reported by the controller, not writable
	puts          int            // successful PUTs
	lastPut       map[string]any // body of the most recent PUT
	failPut       bool           // answer PUT with HTTP 200 and a controller error
	clearOnOff    bool           // a disabled setting keeps no selection
	reverseOnRead bool           // report the ID lists in reverse order
}

func newFakeUpnp(t *testing.T, seed func(f *fakeUpnp)) (*fakeUpnp, *acctest.TestServer) {
	f := &fakeUpnp{}
	if seed != nil {
		seed(f)
	}
	ts := acctest.NewTestServer(t)
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/upnp", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		// Like the live controller, leave out an empty selection.
		result := map[string]any{"enable": f.enable, "supportByDsLiteAndMapE": f.supportDsLite}
		if len(f.networkIds) > 0 {
			result["networkIds"] = f.ordered(f.networkIds)
		}
		if len(f.wanPortIds) > 0 {
			result["wanPortIds"] = f.ordered(f.wanPortIds)
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": result})
	})
	ts.Mux.HandleFunc("PUT /openapi/v1/{omadacId}/sites/{siteId}/upnp", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failPut {
			write(w, map[string]any{"errorCode": -33474, "msg": "This feature is not supported for the DS-Lite or Map-E WAN connection types."})
			return
		}
		f.puts++
		f.lastPut = body
		f.enable, _ = body["enable"].(bool)
		f.networkIds = toStrings(body["networkIds"])
		f.wanPortIds = toStrings(body["wanPortIds"])
		if f.clearOnOff && !f.enable {
			f.networkIds, f.wanPortIds = nil, nil
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeUpnp) ordered(ids []string) []string {
	out := slices.Clone(ids)
	sort.Strings(out)
	if f.reverseOnRead {
		slices.Reverse(out)
	}
	return out
}

func (f *fakeUpnp) set(fn func(f *fakeUpnp)) func() {
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		fn(f)
	}
}

func (f *fakeUpnp) check(fn func(f *fakeUpnp) error) resource.TestCheckFunc {
	return func(*terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		return fn(f)
	}
}

func toStrings(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// sentIds returns the sorted IDs of a PUT body field, and whether the field
// was sent at all.
func sentIds(body map[string]any, key string) ([]string, bool) {
	raw, ok := body[key]
	if !ok {
		return nil, false
	}
	ids := toStrings(raw)
	sort.Strings(ids)
	return ids, true
}

// upnpConfig renders the resource with the given attribute lines.
func upnpConfig(attrs string) string {
	return fmt.Sprintf(`
resource "omada_upnp_setting" "test" {
	site_id = "test-site-id"
%s
}
`, attrs)
}

const disabledConfig = `
	enable       = false
	network_ids  = []
	wan_port_ids = []
`

// The live setting, disabled with nothing selected, imports through an import
// block to a no-op plan.
func TestAcc_UpnpImportBlockIsNoOp(t *testing.T) {
	f, ts := newFakeUpnp(t, nil)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:          ts.ProviderConfig + upnpConfig(disabledConfig),
				ResourceName:    resourceName,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   "test-site-id",
			},
		},
	})
	if f.puts != 0 {
		t.Fatalf("import sent %d PUTs, want none", f.puts)
	}
}

// After `import`, the configuration that matches the live setting plans no
// change and nothing is written.
func TestAcc_UpnpImportAdoptsLive(t *testing.T) {
	f, ts := newFakeUpnp(t, func(f *fakeUpnp) {
		f.enable = true
		f.networkIds = []string{"net-a", "net-b"}
		f.wanPortIds = []string{"wan-1"}
		f.supportDsLite = true
	})
	enabledConfig := `
	enable       = true
	network_ids  = ["net-b", "net-a"]
	wan_port_ids = ["wan-1"]
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             ts.ProviderConfig + upnpConfig(enabledConfig),
				ResourceName:       resourceName,
				ImportState:        true,
				ImportStateId:      "test-site-id",
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("imported %d instances, want 1", len(states))
					}
					want := map[string]string{
						"site_id": "test-site-id", "enable": "true", "network_ids.#": "2",
						"wan_port_ids.#": "1", "support_by_ds_lite_and_map_e": "true",
					}
					for k, v := range want {
						if got := states[0].Attributes[k]; got != v {
							return fmt.Errorf("imported %s = %q, want %q", k, got, v)
						}
					}
					return nil
				},
			},
			{
				// The controller reports the lists in another order; a set
				// doesn't care.
				PreConfig: f.set(func(f *fakeUpnp) { f.reverseOnRead = true }),
				Config:    ts.ProviderConfig + upnpConfig(enabledConfig),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: f.check(func(f *fakeUpnp) error {
					if f.puts != 0 {
						return fmt.Errorf("adopting the live setting sent %d PUTs, want none", f.puts)
					}
					return nil
				}),
			},
		},
	})
}

// Every write sends the whole setting: a field the configuration leaves out is
// sent with its live value, so the full-replace PUT never clears it.
func TestAcc_UpnpUpdateSendsWholeSetting(t *testing.T) {
	f, ts := newFakeUpnp(t, func(f *fakeUpnp) {
		f.enable = true
		f.networkIds = []string{"net-a", "net-b"}
		f.wanPortIds = []string{"wan-1"}
		f.supportDsLite = true
	})
	wantBody := func(networks []string) resource.TestCheckFunc {
		return f.check(func(f *fakeUpnp) error {
			if f.lastPut["enable"] != true {
				return fmt.Errorf("PUT sent enable=%v, want true", f.lastPut["enable"])
			}
			if got, sent := sentIds(f.lastPut, "networkIds"); !sent || !reflect.DeepEqual(got, networks) {
				return fmt.Errorf("PUT sent networkIds=%v (sent %t), want %v", got, sent, networks)
			}
			if got, sent := sentIds(f.lastPut, "wanPortIds"); !sent || !reflect.DeepEqual(got, []string{"wan-1"}) {
				return fmt.Errorf("PUT sent wanPortIds=%v (sent %t), want the live [wan-1]", got, sent)
			}
			if f.lastPut["supportByDsLiteAndMapE"] != true {
				return fmt.Errorf("PUT sent supportByDsLiteAndMapE=%v, want the live true", f.lastPut["supportByDsLiteAndMapE"])
			}
			if !reflect.DeepEqual(f.wanPortIds, []string{"wan-1"}) {
				return fmt.Errorf("the write cleared the WAN ports: %v", f.wanPortIds)
			}
			return nil
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + upnpConfig(`
	enable      = true
	network_ids = ["net-a"]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "network_ids.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "network_ids.*", "net-a"),
					resource.TestCheckResourceAttr(resourceName, "wan_port_ids.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "wan_port_ids.*", "wan-1"),
					resource.TestCheckResourceAttr(resourceName, "support_by_ds_lite_and_map_e", "true"),
					wantBody([]string{"net-a"}),
				),
			},
			{
				Config: ts.ProviderConfig + upnpConfig(`
	enable      = true
	network_ids = ["net-a", "net-b"]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "network_ids.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "wan_port_ids.#", "1"),
					wantBody([]string{"net-a", "net-b"}),
				),
			},
		},
	})
}

// Turning UPnP off with the selection left out of the configuration works when
// the controller drops the selection of a disabled setting, and declaring the
// empty selection afterwards plans no change.
func TestAcc_UpnpDisable(t *testing.T) {
	f, ts := newFakeUpnp(t, func(f *fakeUpnp) {
		f.enable = true
		f.networkIds = []string{"net-a"}
		f.wanPortIds = []string{"wan-1"}
		f.clearOnOff = true
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + upnpConfig(`
	enable = true
`),
			},
			{
				Config: ts.ProviderConfig + upnpConfig(`
	enable = false
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					resource.TestCheckResourceAttr(resourceName, "network_ids.#", "0"),
					resource.TestCheckResourceAttr(resourceName, "wan_port_ids.#", "0"),
					f.check(func(f *fakeUpnp) error {
						if f.enable {
							return fmt.Errorf("the controller still has UPnP on")
						}
						return nil
					}),
				),
			},
			{
				Config: ts.ProviderConfig + upnpConfig(disabledConfig),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// UPnP turned on in the controller UI shows as drift, and the apply turns it
// back off.
func TestAcc_UpnpDrift(t *testing.T) {
	f, ts := newFakeUpnp(t, nil)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + upnpConfig(disabledConfig)},
			{
				PreConfig: f.set(func(f *fakeUpnp) {
					f.enable = true
					f.networkIds = []string{"net-a"}
					f.wanPortIds = []string{"wan-1"}
				}),
				Config:             ts.ProviderConfig + upnpConfig(disabledConfig),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: ts.ProviderConfig + upnpConfig(disabledConfig),
				Check: f.check(func(f *fakeUpnp) error {
					if f.enable || len(f.networkIds) != 0 || len(f.wanPortIds) != 0 {
						return fmt.Errorf("apply left enable=%t networks=%v wan=%v, want off with nothing selected", f.enable, f.networkIds, f.wanPortIds)
					}
					return nil
				}),
			},
		},
	})
}

// Destroy leaves the live setting alone: it only removes it from state.
func TestAcc_UpnpDestroyLeavesSetting(t *testing.T) {
	f, ts := newFakeUpnp(t, nil)
	var putsBefore int

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + upnpConfig(`
	enable      = true
	network_ids = ["net-a"]
`),
				Check: f.check(func(f *fakeUpnp) error {
					putsBefore = f.puts
					return nil
				}),
			},
		},
		CheckDestroy: f.check(func(f *fakeUpnp) error {
			if f.puts != putsBefore || !f.enable || !reflect.DeepEqual(f.networkIds, []string{"net-a"}) {
				return fmt.Errorf("destroy changed the live setting (PUTs %d -> %d, enable=%t, networks=%v)", putsBefore, f.puts, f.enable, f.networkIds)
			}
			return nil
		}),
	})
}

// A controller error in a 2xx answer fails the apply instead of being recorded
// as success, and a malformed import ID is refused.
func TestAcc_UpnpErrors(t *testing.T) {
	f, ts := newFakeUpnp(t, func(f *fakeUpnp) { f.failPut = true })

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + upnpConfig(disabledConfig),
				ExpectError: regexp.MustCompile(`(?s)creating UPnP setting.*-33474`),
			},
			{
				PreConfig:     f.set(func(f *fakeUpnp) { f.failPut = false }),
				Config:        ts.ProviderConfig + upnpConfig(disabledConfig),
				ResourceName:  resourceName,
				ImportState:   true,
				ImportStateId: "test-site-id/extra",
				ExpectError:   regexp.MustCompile(`Unexpected Import ID`),
			},
		},
	})
}
