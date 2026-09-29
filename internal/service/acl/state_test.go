package acl_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// fakeAclController models the parts of the controller that state accuracy
// depends on: a list that is the only way to read a rule, a
// DELETE that fails with a controller error when the rule is already gone, and
// switches to delete a rule behind Terraform's back or to hide it from lists.
type fakeAclController struct {
	mu     sync.Mutex
	rows   map[string]map[string]any
	hidden bool // list reads omit every row (a read-back that never converges)
	// vanishOnDelete removes the rule just as the DELETE arrives and answers
	// with a controller error, as when someone deletes it in the UI between
	// Terraform's refresh and its delete.
	vanishOnDelete bool
	posts          int
}

func newFakeAclController(t *testing.T) (*fakeAclController, *acctest.TestServer) {
	f := &fakeAclController{rows: map[string]map[string]any{}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	ts.Mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts++
		id := "acl-" + string(rune('0'+f.posts))
		body["id"] = id
		body["index"] = len(f.rows) + 1
		f.rows[id] = body
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"id": id}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data := []any{}
		if !f.hidden {
			for _, row := range f.rows {
				data = append(data, row)
			}
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"totalRows": len(data), "currentPage": 1, "currentSize": 1000, "data": data,
		}})
	})
	ts.Mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/acls/{aclId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("aclId")
		if f.vanishOnDelete {
			delete(f.rows, id)
		}
		if _, ok := f.rows[id]; !ok || f.vanishOnDelete {
			// The real not-found code for this endpoint is undocumented; any
			// controller error must be confirmed against the list.
			write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		delete(f.rows, id)
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeAclController) deleteAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = map[string]map[string]any{}
}

func (f *fakeAclController) setHidden(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hidden = v
}

const stateTestAcl = `
resource "omada_acl" "test" {
	site_id          = "test-site-id"
	description      = "Untrusted deny"
	source_type      = 0
	source_ids       = ["net-a"]
	destination_type = 0
	destination_ids  = ["net-b"]
	policy           = 0
	protocols        = [1, 6, 17]
	state_mode       = 0
	status           = true
	syslog           = true
	direction = {
		lan_to_lan = true
	}
}
`

// A rule deleted in the controller UI must show up as drift. On 0.14.0 Read
// left the framework's pre-filled prior state in place, so the plan was empty
// and a deleted deny rule looked compliant.
func TestAcc_AclDeletedOutOfBand(t *testing.T) {
	f, ts := newFakeAclController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestAcl},
			{
				PreConfig:          f.deleteAll,
				Config:             ts.ProviderConfig + stateTestAcl,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// The apply recreates the rule.
				Config: ts.ProviderConfig + stateTestAcl,
				Check:  resource.TestCheckResourceAttr("omada_acl.test", "acl_id", "acl-2"),
			},
		},
	})
}

// Destroying a rule that is already gone must succeed: the controller's error
// is checked against the list, which no longer has the rule.
func TestAcc_AclDestroyAlreadyGone(t *testing.T) {
	f, ts := newFakeAclController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestAcl},
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
func TestAcc_AclImportMissing(t *testing.T) {
	_, ts := newFakeAclController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        ts.ProviderConfig + stateTestAcl,
				ResourceName:  "omada_acl.test",
				ImportState:   true,
				ImportStateId: "test-site-id/does-not-exist",
				ExpectError:   regexp.MustCompile(`(?i)cannot import non-existent remote object`),
			},
		},
	})
}

// When the read-back after a successful POST never converges, the created rule
// must stay in state (tainted), so the next plan replaces it instead of
// creating a duplicate next to an unmanaged original.
func TestAcc_AclCreateReadBackFailsKeepsState(t *testing.T) {
	f, ts := newFakeAclController(t)
	f.setHidden(true)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + stateTestAcl,
				ExpectError: regexp.MustCompile(`kept in\s+state as tainted`),
			},
			{
				PreConfig: func() { f.setHidden(false) },
				Config:    ts.ProviderConfig + stateTestAcl,
				// The tainted rule is replaced: the original acl-1 is deleted and
				// acl-2 created, never a second rule beside an orphan.
				Check: resource.TestCheckResourceAttr("omada_acl.test", "acl_id", "acl-2"),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, orphan := f.rows["acl-1"]; orphan {
		t.Fatal("the rule created by the failed apply was orphaned on the controller")
	}
}
