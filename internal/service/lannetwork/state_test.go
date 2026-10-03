package lannetwork_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// fakeLanController models the parts of the controller that state accuracy
// depends on: a list that is the only way to read a network, a
// DELETE that fails when the network is already gone, and switches to delete a
// network behind Terraform's back, hide it from lists, or omit the id from the
// create result.
type fakeLanController struct {
	mu     sync.Mutex
	rows   map[string]map[string]any
	hidden bool // list reads omit every row (a read-back that never converges)
	// vanishOnDelete removes the network just as the DELETE arrives and answers
	// with a generic controller error rather than -33503, as when someone
	// deletes it in the UI between Terraform's refresh and its delete.
	vanishOnDelete bool
	// noIdOnCreate answers the POST with an empty envelope, as the live
	// controller does, so the resource must find the network by name.
	noIdOnCreate bool
	// dhcpDefault makes a create without DHCP settings read back the way the
	// controller reports it: dhcpSettingsVO {enable: false, ...defaults}.
	dhcpDefault bool
	posts       int
	patches     int
	lastPatch   map[string]any // the body of the most recent update
}

func newFakeLanController(t *testing.T) (*fakeLanController, *acctest.TestServer) {
	f := &fakeLanController{rows: map[string]map[string]any{}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	ts.Mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/lan-networks", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts++
		id := fmt.Sprintf("net-%d", f.posts)
		body["id"] = id
		if _, ok := body["dhcpSettingsVO"]; !ok && f.dhcpDefault {
			body["dhcpSettingsVO"] = map[string]any{"enable": false, "dhcpns": "auto", "leasetime": 120, "options": []any{}}
		}
		f.rows[id] = body
		if f.noIdOnCreate {
			write(w, map[string]any{"errorCode": 0, "msg": "Success."})
			return
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"id": id}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/lan-networks", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data := []any{}
		if !f.hidden {
			for _, row := range f.rows {
				// Like the live controller, the list shows the default
				// network's stored name with a display suffix.
				if primary, _ := row["primary"].(bool); primary {
					shown := map[string]any{}
					for k, v := range row {
						shown[k] = v
					}
					shown["name"] = fmt.Sprint(row["name"]) + "(Default)"
					row = shown
				}
				data = append(data, row)
			}
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"totalRows": len(data), "currentPage": 1, "currentSize": 1000, "data": data,
		}})
	})
	ts.Mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/lan-networks/{networkId}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		row, ok := f.rows[r.PathValue("networkId")]
		if !ok {
			write(w, map[string]any{"errorCode": -33503, "msg": "The LAN network does not exist."})
			return
		}
		f.patches++
		f.lastPatch = body
		for k, v := range body {
			row[k] = v
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	ts.Mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/lan-networks/{networkId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("networkId")
		if f.vanishOnDelete {
			delete(f.rows, id)
			// Not the documented -33503: the resource must confirm absence
			// against the list.
			write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		if _, ok := f.rows[id]; !ok {
			write(w, map[string]any{"errorCode": -33503, "msg": "The LAN network does not exist."})
			return
		}
		delete(f.rows, id)
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeLanController) deleteAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = map[string]map[string]any{}
}

func (f *fakeLanController) setHidden(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hidden = v
}

const stateTestLanNetwork = `
resource "omada_lan_network" "test" {
	site_id        = "test-site-id"
	name           = "Untrusted"
	vlan_id        = 40
	gateway_subnet = "192.168.120.1/24"
	interface_ids  = ["port-1"]
}
`

// A network deleted in the controller UI must show up as drift. Read used to
// leave the framework's pre-filled prior state in place, so the plan was empty.
func TestAcc_LanNetworkDeletedOutOfBand(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestLanNetwork},
			{
				PreConfig:          f.deleteAll,
				Config:             ts.ProviderConfig + stateTestLanNetwork,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// The apply recreates the network.
				Config: ts.ProviderConfig + stateTestLanNetwork,
				Check:  resource.TestCheckResourceAttr("omada_lan_network.test", "network_id", "net-2"),
			},
		},
	})
}

// Destroying a network that is already gone must succeed even when the
// controller answers with a code other than -33503: the error is checked
// against the list, which no longer has the network.
func TestAcc_LanNetworkDestroyAlreadyGone(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestLanNetwork},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					f.vanishOnDelete = true
				},
				Config: ts.ProviderConfig,
			},
		},
	})
}

// Importing an id that doesn't exist must fail, not import a half-empty state.
func TestAcc_LanNetworkImportMissing(t *testing.T) {
	_, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        ts.ProviderConfig + stateTestLanNetwork,
				ResourceName:  "omada_lan_network.test",
				ImportState:   true,
				ImportStateId: "test-site-id/does-not-exist",
				ExpectError:   regexp.MustCompile(`(?i)cannot import non-existent remote object`),
			},
		},
	})
}

// A create result without an id must fall back to the name lookup. network_id
// is Unknown (not Null) at create, so the old IsNull guard skipped the lookup
// and stored a null network_id.
func TestAcc_LanNetworkCreateWithoutId(t *testing.T) {
	f, ts := newFakeLanController(t)
	f.noIdOnCreate = true

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + stateTestLanNetwork,
				Check:  resource.TestCheckResourceAttr("omada_lan_network.test", "network_id", "net-1"),
			},
		},
	})
}

// When the read-back after a successful POST never converges, the created
// network must stay in state (tainted), so the next plan replaces it instead
// of creating a duplicate next to an unmanaged original.
func TestAcc_LanNetworkCreateReadBackFailsKeepsState(t *testing.T) {
	f, ts := newFakeLanController(t)
	f.setHidden(true)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + stateTestLanNetwork,
				ExpectError: regexp.MustCompile(`kept in\s+state as tainted`),
			},
			{
				PreConfig: func() { f.setHidden(false) },
				Config:    ts.ProviderConfig + stateTestLanNetwork,
				// The tainted network is replaced: the original net-1 is deleted
				// and net-2 created, never a second network beside an orphan.
				Check: resource.TestCheckResourceAttr("omada_lan_network.test", "network_id", "net-2"),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, orphan := f.rows["net-1"]; orphan {
		t.Fatal("the network created by the failed apply was orphaned on the controller")
	}
}

func (f *fakeLanController) set(fn func(f *fakeLanController)) func() {
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		fn(f)
	}
}
