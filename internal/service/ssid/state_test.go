package ssid_test

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

const (
	stateTestGroup = "test-wlan-group-id"
	// codeInvalidParams is the controller's generic "invalid request
	// parameters", which 5.15.x also returns for a missing SSID.
	codeInvalidParams = -1001
	// codeSsidGone stands in for a not-found code other than -1001: Delete must
	// confirm any controller error against the list rather than trust a code.
	codeSsidGone = -33803
)

// fakeSsidController models the parts of the controller that state accuracy
// depends on (homelab #514): a detail GET and a list per WLAN group, a detail
// GET and DELETE that answer the generic -1001 in several situations, and
// switches to delete an SSID or its whole group behind Terraform's back.
type fakeSsidController struct {
	mu     sync.Mutex
	rows   map[string]map[string]any
	groups map[string]bool
	hidden bool // detail reads refuse and list reads omit every SSID
	// refuseDetail makes the detail GET answer -1001 while the SSID stays
	// listed: -1001 alone is not proof of absence.
	refuseDetail bool
	// rejectDelete answers DELETE with -1001 and keeps the SSID.
	rejectDelete bool
	// vanishOnDelete removes the SSID just as the DELETE arrives and answers
	// with a controller error, as when someone deletes it in the UI between
	// Terraform's refresh and its delete.
	vanishOnDelete bool
	omitCreateID   bool // the create result carries no id
	badGateways    int  // the next detail reads answer a gateway error page
	posts          int
	lastPatch      map[string]any // the body of the most recent update
}

func newFakeSsidController(t *testing.T) (*fakeSsidController, *acctest.TestServer) {
	f := &fakeSsidController{rows: map[string]map[string]any{}, groups: map[string]bool{stateTestGroup: true}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	invalid := map[string]any{"errorCode": codeInvalidParams, "msg": "Invalid request parameters."}
	const base = "/openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans"

	ts.Mux.HandleFunc("GET "+base, func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data := []any{}
		for id := range f.groups {
			data = append(data, map[string]any{"wlanId": id, "name": id, "primary": false})
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": data})
	})
	ts.Mux.HandleFunc("POST "+base+"/{wlanId}/ssids", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts++
		id := fmt.Sprintf("ssid-%d", f.posts)
		body["ssidId"] = id
		f.rows[id] = body
		if f.omitCreateID {
			write(w, map[string]any{"errorCode": 0, "msg": "Success."})
			return
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"ssidId": id}})
	})
	ts.Mux.HandleFunc("GET "+base+"/{wlanId}/ssids", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.groups[r.PathValue("wlanId")] {
			write(w, invalid)
			return
		}
		data := []any{}
		if !f.hidden {
			for _, row := range f.rows {
				data = append(data, map[string]any{"ssidId": row["ssidId"], "name": row["name"]})
			}
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"totalRows": len(data), "currentPage": 1, "currentSize": 1000, "data": data,
		}})
	})
	ts.Mux.HandleFunc("GET "+base+"/{wlanId}/ssids/{ssidId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.badGateways > 0 {
			f.badGateways--
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
			return
		}
		row, ok := f.rows[r.PathValue("ssidId")]
		if !ok || f.hidden || f.refuseDetail || !f.groups[r.PathValue("wlanId")] {
			write(w, invalid)
			return
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": row})
	})
	ts.Mux.HandleFunc("PATCH "+base+"/{wlanId}/ssids/{ssidId}/update-basic-config", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		row, ok := f.rows[r.PathValue("ssidId")]
		if !ok {
			write(w, invalid)
			return
		}
		f.lastPatch = body
		for k, v := range body {
			row[k] = v
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	ts.Mux.HandleFunc("DELETE "+base+"/{wlanId}/ssids/{ssidId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("ssidId")
		if f.rejectDelete {
			write(w, invalid)
			return
		}
		if f.vanishOnDelete {
			delete(f.rows, id)
		}
		if _, ok := f.rows[id]; !ok || f.vanishOnDelete {
			write(w, map[string]any{"errorCode": codeSsidGone, "msg": "The SSID does not exist."})
			return
		}
		delete(f.rows, id)
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeSsidController) set(fn func(f *fakeSsidController)) func() {
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		fn(f)
	}
}

const stateTestSsid = `
resource "omada_ssid" "test" {
	site_id       = "test-site-id"
	wlan_group_id = "test-wlan-group-id"
	name          = "TF State SSID"
	security      = 3
	band          = 3

	psk_setting = {
		psk = "state-test-psk-value"
	}
}
`

// An SSID deleted in the controller UI must show up as drift. Read used to
// leave the framework's pre-filled prior state in place, so the plan was empty.
func TestAcc_SsidDeletedOutOfBand(t *testing.T) {
	f, ts := newFakeSsidController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestSsid},
			{
				PreConfig:          f.set(func(f *fakeSsidController) { f.rows = map[string]map[string]any{} }),
				Config:             ts.ProviderConfig + stateTestSsid,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// The apply recreates the SSID.
				Config: ts.ProviderConfig + stateTestSsid,
				Check:  resource.TestCheckResourceAttr("omada_ssid.test", "ssid_id", "ssid-2"),
			},
		},
	})
}

// Deleting the WLAN group out of band takes its SSIDs with it; the SSID list
// of a missing group is rejected, and that must read as drift, not an error.
func TestAcc_SsidWlanGroupDeletedOutOfBand(t *testing.T) {
	f, ts := newFakeSsidController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestSsid},
			{
				PreConfig: f.set(func(f *fakeSsidController) {
					f.rows = map[string]map[string]any{}
					delete(f.groups, stateTestGroup)
				}),
				Config:             ts.ProviderConfig + stateTestSsid,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				PreConfig: f.set(func(f *fakeSsidController) { f.groups[stateTestGroup] = true }),
				Config:    ts.ProviderConfig + stateTestSsid,
				Check:     resource.TestCheckResourceAttr("omada_ssid.test", "ssid_id", "ssid-2"),
			},
		},
	})
}

// -1001 on the detail read is the controller's generic "invalid request
// parameters". While the SSID is still listed, Read must fail rather than
// drop (or keep stale) a live SSID.
func TestAcc_SsidDetailRefusedButListed(t *testing.T) {
	f, ts := newFakeSsidController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestSsid},
			{
				PreConfig:   f.set(func(f *fakeSsidController) { f.refuseDetail = true }),
				Config:      ts.ProviderConfig + stateTestSsid,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`still listed in WLAN group`),
			},
			{
				PreConfig: f.set(func(f *fakeSsidController) { f.refuseDetail = false }),
				Config:    ts.ProviderConfig + stateTestSsid,
				PlanOnly:  true,
			},
		},
	})
}

// Destroying an SSID that is already gone must succeed: the controller's error
// is checked against the list, which no longer has the SSID.
func TestAcc_SsidDestroyAlreadyGone(t *testing.T) {
	f, ts := newFakeSsidController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestSsid},
			{
				PreConfig: f.set(func(f *fakeSsidController) { f.vanishOnDelete = true }),
				Config:    ts.ProviderConfig,
			},
		},
	})
}

// A delete the controller rejects with -1001 while the SSID stays live must
// fail, not drop the SSID from state and leave it broadcasting unmanaged.
func TestAcc_SsidDestroyRejectedWhileListed(t *testing.T) {
	f, ts := newFakeSsidController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestSsid},
			{
				PreConfig:   f.set(func(f *fakeSsidController) { f.rejectDelete = true }),
				Config:      ts.ProviderConfig,
				ExpectError: regexp.MustCompile(`Error deleting SSID`),
			},
			{
				// Still in state: the plan against the original config is empty.
				PreConfig: f.set(func(f *fakeSsidController) { f.rejectDelete = false }),
				Config:    ts.ProviderConfig + stateTestSsid,
				PlanOnly:  true,
			},
		},
	})
}

// Importing an id that doesn't exist must fail, not import a half-empty state.
func TestAcc_SsidImportMissing(t *testing.T) {
	_, ts := newFakeSsidController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        ts.ProviderConfig + stateTestSsid,
				ResourceName:  "omada_ssid.test",
				ImportState:   true,
				ImportStateId: "test-site-id/test-wlan-group-id/does-not-exist",
				ExpectError:   regexp.MustCompile(`(?i)cannot import non-existent remote object`),
			},
		},
	})
}

// When the read-back after a successful POST never converges, the created SSID
// must stay in state (tainted), so the next plan replaces it instead of
// creating a second SSID next to an unmanaged original.
func TestAcc_SsidCreateReadBackFailsKeepsState(t *testing.T) {
	f, ts := newFakeSsidController(t)
	f.set(func(f *fakeSsidController) { f.hidden = true })()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + stateTestSsid,
				ExpectError: regexp.MustCompile(`kept in\s+state as tainted`),
			},
			{
				PreConfig: f.set(func(f *fakeSsidController) { f.hidden = false }),
				Config:    ts.ProviderConfig + stateTestSsid,
				// The tainted SSID is replaced: ssid-1 is deleted and ssid-2
				// created, never a second SSID beside an orphan.
				Check: resource.TestCheckResourceAttr("omada_ssid.test", "ssid_id", "ssid-2"),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, orphan := f.rows["ssid-1"]; orphan {
		t.Fatal("the SSID created by the failed apply was orphaned on the controller")
	}
}

// A transient failure of the read-back followed by a success must not fail
// the apply: only the last attempt's diagnostics count.
func TestAcc_SsidCreateTransientReadBackError(t *testing.T) {
	f, ts := newFakeSsidController(t)
	f.set(func(f *fakeSsidController) { f.badGateways = 1 })()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + stateTestSsid,
				Check:  resource.TestCheckResourceAttr("omada_ssid.test", "ssid_id", "ssid-1"),
			},
		},
	})
}

// When the create result carries no id, the SSID is found by name. ssid_id is
// Unknown (not Null) at create time, which once skipped the lookup.
func TestAcc_SsidCreateWithoutIdFindsByName(t *testing.T) {
	f, ts := newFakeSsidController(t)
	f.set(func(f *fakeSsidController) { f.omitCreateID = true })()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + stateTestSsid,
				Check:  resource.TestCheckResourceAttr("omada_ssid.test", "ssid_id", "ssid-1"),
			},
		},
	})
}
