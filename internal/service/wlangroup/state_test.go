package wlangroup_test

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
	// codeInvalidParams is the controller's generic "invalid request
	// parameters", which 5.15.x also returns for a missing WLAN group.
	codeInvalidParams = -1001
	// codeGroupGone stands in for a not-found code other than -1001: Delete
	// must confirm any controller error against the list rather than trust a
	// code.
	codeGroupGone = -33804
)

// fakeWlanGroupController models the parts of the controller that state
// accuracy depends on: a list that is the only way to read a
// group, a DELETE that answers -1001 both for a missing group and for a
// rejected one, and switches to delete a group behind Terraform's back or to
// hide it from lists.
type fakeWlanGroupController struct {
	mu     sync.Mutex
	rows   map[string]map[string]any
	hidden bool // list reads omit every group (a read-back that never converges)
	// rejectDelete answers DELETE with -1001 and keeps the group, as the
	// controller does for a group it refuses to delete.
	rejectDelete bool
	// vanishOnDelete removes the group just as the DELETE arrives and answers
	// with a controller error, as when someone deletes it in the UI between
	// Terraform's refresh and its delete.
	vanishOnDelete bool
	omitCreateID   bool // the create result carries no id
	badGateways    int  // the next list reads answer a gateway error page
	posts          int
}

func newFakeWlanGroupController(t *testing.T) (*fakeWlanGroupController, *acctest.TestServer) {
	f := &fakeWlanGroupController{rows: map[string]map[string]any{
		"default-group-id": {"wlanId": "default-group-id", "name": "Default", "primary": true},
	}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	invalid := map[string]any{"errorCode": codeInvalidParams, "msg": "Invalid request parameters."}
	const base = "/openapi/v1/{omadacId}/sites/{siteId}/wireless-network/wlans"

	ts.Mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts++
		id := fmt.Sprintf("wlan-%d", f.posts)
		f.rows[id] = map[string]any{"wlanId": id, "name": body["name"], "primary": false}
		if f.omitCreateID {
			write(w, map[string]any{"errorCode": 0, "msg": "Success."})
			return
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"wlanId": id}})
	})
	ts.Mux.HandleFunc("GET "+base, func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.badGateways > 0 {
			f.badGateways--
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
			return
		}
		data := []any{}
		if !f.hidden {
			for _, row := range f.rows {
				data = append(data, row)
			}
		}
		// The 5.15.x list is a bare array under "result".
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": data})
	})
	ts.Mux.HandleFunc("PATCH "+base+"/{wlanId}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		row, ok := f.rows[r.PathValue("wlanId")]
		if !ok {
			write(w, invalid)
			return
		}
		row["name"] = body["name"]
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	ts.Mux.HandleFunc("DELETE "+base+"/{wlanId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("wlanId")
		if f.rejectDelete {
			write(w, invalid)
			return
		}
		if f.vanishOnDelete {
			delete(f.rows, id)
		}
		if _, ok := f.rows[id]; !ok || f.vanishOnDelete {
			write(w, map[string]any{"errorCode": codeGroupGone, "msg": "The WLAN group does not exist."})
			return
		}
		delete(f.rows, id)
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeWlanGroupController) set(fn func(f *fakeWlanGroupController)) func() {
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		fn(f)
	}
}

const stateTestWlanGroup = `
resource "omada_wlan_group" "test" {
	site_id = "test-site-id"
	name    = "TF State WLAN Group"
}
`

// A group deleted in the controller UI must show up as drift. Read used to
// leave the framework's pre-filled prior state in place, so the plan was empty.
func TestAcc_WlanGroupDeletedOutOfBand(t *testing.T) {
	f, ts := newFakeWlanGroupController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestWlanGroup},
			{
				PreConfig:          f.set(func(f *fakeWlanGroupController) { delete(f.rows, "wlan-1") }),
				Config:             ts.ProviderConfig + stateTestWlanGroup,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// The apply recreates the group.
				Config: ts.ProviderConfig + stateTestWlanGroup,
				Check:  resource.TestCheckResourceAttr("omada_wlan_group.test", "wlan_group_id", "wlan-2"),
			},
		},
	})
}

// Destroying a group that is already gone must succeed: the controller's error
// is checked against the list, which no longer has the group.
func TestAcc_WlanGroupDestroyAlreadyGone(t *testing.T) {
	f, ts := newFakeWlanGroupController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestWlanGroup},
			{
				PreConfig: f.set(func(f *fakeWlanGroupController) { f.vanishOnDelete = true }),
				Config:    ts.ProviderConfig,
			},
		},
	})
}

// A delete the controller rejects with -1001 while the group stays live must
// fail, not drop the group from state.
func TestAcc_WlanGroupDestroyRejectedWhileListed(t *testing.T) {
	f, ts := newFakeWlanGroupController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestWlanGroup},
			{
				PreConfig:   f.set(func(f *fakeWlanGroupController) { f.rejectDelete = true }),
				Config:      ts.ProviderConfig,
				ExpectError: regexp.MustCompile(`Error deleting WLAN group`),
			},
			{
				// Still in state: the plan against the original config is empty.
				PreConfig: f.set(func(f *fakeWlanGroupController) { f.rejectDelete = false }),
				Config:    ts.ProviderConfig + stateTestWlanGroup,
				PlanOnly:  true,
			},
		},
	})
}

// Importing an id that doesn't exist must fail, not import a half-empty state.
func TestAcc_WlanGroupImportMissing(t *testing.T) {
	_, ts := newFakeWlanGroupController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        ts.ProviderConfig + stateTestWlanGroup,
				ResourceName:  "omada_wlan_group.test",
				ImportState:   true,
				ImportStateId: "test-site-id/does-not-exist",
				ExpectError:   regexp.MustCompile(`(?i)cannot import non-existent remote object`),
			},
		},
	})
}

// When the read-back after a successful POST never converges, the created
// group must stay in state (tainted), so the next plan replaces it instead of
// failing on a duplicate name next to an unmanaged original.
func TestAcc_WlanGroupCreateReadBackFailsKeepsState(t *testing.T) {
	f, ts := newFakeWlanGroupController(t)
	f.set(func(f *fakeWlanGroupController) { f.hidden = true })()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + stateTestWlanGroup,
				ExpectError: regexp.MustCompile(`kept\s+in\s+state as tainted`),
			},
			{
				PreConfig: f.set(func(f *fakeWlanGroupController) { f.hidden = false }),
				Config:    ts.ProviderConfig + stateTestWlanGroup,
				// The tainted group is replaced: wlan-1 is deleted and wlan-2
				// created, never a second group beside an orphan.
				Check: resource.TestCheckResourceAttr("omada_wlan_group.test", "wlan_group_id", "wlan-2"),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, orphan := f.rows["wlan-1"]; orphan {
		t.Fatal("the group created by the failed apply was orphaned on the controller")
	}
}

// A transient failure of the read-back followed by a success must not fail
// the apply: only the last attempt's diagnostics count.
func TestAcc_WlanGroupCreateTransientReadBackError(t *testing.T) {
	f, ts := newFakeWlanGroupController(t)
	f.set(func(f *fakeWlanGroupController) { f.badGateways = 1 })()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + stateTestWlanGroup,
				Check:  resource.TestCheckResourceAttr("omada_wlan_group.test", "wlan_group_id", "wlan-1"),
			},
		},
	})
}

// When the create result carries no id, the group is found by name.
// wlan_group_id is Unknown (not Null) at create time, which once skipped the
// lookup.
func TestAcc_WlanGroupCreateWithoutIdFindsByName(t *testing.T) {
	f, ts := newFakeWlanGroupController(t)
	f.set(func(f *fakeWlanGroupController) { f.omitCreateID = true })()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + stateTestWlanGroup,
				Check:  resource.TestCheckResourceAttr("omada_wlan_group.test", "wlan_group_id", "wlan-1"),
			},
		},
	})
}
