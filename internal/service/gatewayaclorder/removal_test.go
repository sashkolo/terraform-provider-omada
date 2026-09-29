package gatewayaclorder_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeGatewayAcls models the controller's gateway ACL list for the ACL and
// order resources together: rules are created, reordered and deleted, and a
// rule stays in the list until its DELETE arrives.
type fakeGatewayAcls struct {
	mu    sync.Mutex
	rows  map[string]map[string]any
	order []string
	posts int
}

func newFakeGatewayAcls(t *testing.T) (*fakeGatewayAcls, *acctest.TestServer) {
	f := &fakeGatewayAcls{rows: map[string]map[string]any{}}
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
		id := fmt.Sprintf("acl-%d", f.posts)
		body["id"] = id
		f.rows[id] = body
		f.order = append(f.order, id) // a new rule lands at the bottom
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"id": id}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data := []any{}
		for i, id := range f.order {
			row := map[string]any{}
			for k, v := range f.rows[id] {
				row[k] = v
			}
			row["index"] = i + 1
			data = append(data, row)
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"totalRows": len(data), "currentPage": 1, "currentSize": 1000, "data": data,
		}})
	})
	ts.Mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/acls/modifyIndex", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Indexes map[string]int32 `json:"indexes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		// The controller reorders only whole maps of existing rules.
		if len(body.Indexes) != len(f.order) {
			write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		next := make([]string, 0, len(body.Indexes))
		for id := range body.Indexes {
			if _, ok := f.rows[id]; !ok {
				write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
				return
			}
			next = append(next, id)
		}
		sort.SliceStable(next, func(i, j int) bool { return body.Indexes[next[i]] < body.Indexes[next[j]] })
		f.order = next
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	ts.Mux.HandleFunc("PUT /openapi/v1/{omadacId}/sites/{siteId}/acls/osg-acls/{aclId}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("aclId")
		if _, ok := f.rows[id]; !ok {
			write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		body["id"] = id
		f.rows[id] = body
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	ts.Mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/acls/{aclId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("aclId")
		delete(f.rows, id)
		for i, o := range f.order {
			if o == id {
				f.order = append(f.order[:i], f.order[i+1:]...)
				break
			}
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func removalAcl(name, description string) string {
	return fmt.Sprintf(`
resource "omada_acl" %q {
	site_id          = "test-site-id"
	description      = %q
	status           = true
	syslog           = false
	source_type      = 0
	source_ids       = ["net-guest"]
	destination_type = 0
	destination_ids  = ["net-%s"]
	policy           = 0
	protocols        = [1, 6, 17]
	state_mode       = 0
	direction = {
		lan_to_lan = true
	}
}
`, name, description, name)
}

// Removing an ACL and its id from ordered_acl_ids in one apply must work. The
// order's new configuration no longer references the removed ACL, so the engine
// destroys the ACL first and then updates the order, whose exhaustiveness check
// no longer sees it.
func TestAcc_GatewayAclOrderRemovesAclInSameApply(t *testing.T) {
	f, ts := newFakeGatewayAcls(t)

	both := ts.ProviderConfig + removalAcl("probe", "Transition probe") + removalAcl("deny", "Guest deny") + `
resource "omada_gateway_acl_order" "test" {
	site_id         = "test-site-id"
	ordered_acl_ids = [omada_acl.probe.acl_id, omada_acl.deny.acl_id]
}
`
	denyOnly := ts.ProviderConfig + removalAcl("deny", "Guest deny") + `
resource "omada_gateway_acl_order" "test" {
	site_id         = "test-site-id"
	ordered_acl_ids = [omada_acl.deny.acl_id]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: both},
			{
				// Retire the probe: its resource and its id go in one apply.
				Config: denyOnly,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gateway_acl_order.test", "ordered_acl_ids.#", "1"),
					resource.TestCheckResourceAttrPair("omada_gateway_acl_order.test", "ordered_acl_ids.0", "omada_acl.deny", "acl_id"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if len(f.rows) != 1 || len(f.order) != 1 {
							return fmt.Errorf("%d ACLs left on the controller, want only the deny", len(f.rows))
						}
						return nil
					},
				),
			},
		},
	})
}

// A rule created outside Terraform must still be refused: the removal fix must
// not weaken the exhaustiveness guard.
func TestAcc_GatewayAclOrderStillRefusesUnmanagedAcl(t *testing.T) {
	f, ts := newFakeGatewayAcls(t)

	config := ts.ProviderConfig + removalAcl("deny", "Guest deny") + `
resource "omada_gateway_acl_order" "test" {
	site_id         = "test-site-id"
	ordered_acl_ids = [omada_acl.deny.acl_id]
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				// Someone adds a rule in the UI.
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					f.rows["ui-rule"] = map[string]any{"id": "ui-rule", "description": "UI rule"}
					f.order = append([]string{"ui-rule"}, f.order...)
				},
				// A config change forces the order resource to apply again.
				Config: ts.ProviderConfig + removalAcl("deny", "Guest deny, renamed") + `
resource "omada_gateway_acl_order" "test" {
	site_id         = "test-site-id"
	ordered_acl_ids = [omada_acl.deny.acl_id]
}
`,
				ExpectError: regexp.MustCompile(`not exhaustive|ui-rule`),
			},
		},
	})
}
