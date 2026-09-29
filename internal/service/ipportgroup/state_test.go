package ipportgroup_test

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

// fakeGroupController models the parts of the controller that state accuracy
// depends on (homelab #514): a per-type list that is the only way to read a
// group and that is eventually consistent after a write.
type fakeGroupController struct {
	mu   sync.Mutex
	rows map[string]map[string]any
	// hideReads makes that many list reads omit every row, as the list does
	// for a moment after a write; hideAll omits them until cleared.
	hideReads int
	hideAll   bool
	posts     int
}

func newFakeGroupController(t *testing.T) (*fakeGroupController, *acctest.TestServer) {
	f := &fakeGroupController{rows: map[string]map[string]any{}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	ts.Mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts++
		id := fmt.Sprintf("group-%d", f.posts)
		body["groupId"] = id
		f.rows[id] = body
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"id": id}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data := []any{}
		switch {
		case f.hideAll:
		case f.hideReads > 0:
			f.hideReads--
		default:
			for _, row := range f.rows {
				data = append(data, row)
			}
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": data})
	})
	ts.Mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}/{groupId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("groupId")
		if _, ok := f.rows[id]; !ok {
			write(w, map[string]any{"errorCode": -33704, "msg": "This group does not exist."})
			return
		}
		delete(f.rows, id)
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeGroupController) set(fn func(f *fakeGroupController)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

const stateTestGroup = `
resource "omada_ip_port_group" "test" {
	site_id = "test-site-id"
	name    = "camera-service-ports"
	ip_list = [
		{ ip = "192.168.30.0", mask = 24 },
	]
	port_list = ["554"]
}
`

// A read-back that misses the new group for a moment must not null the id the
// POST returned. On 0.14.0 the first miss saved group_id = null, and an ACL
// referencing the group then carried a null element.
func TestAcc_IPPortGroupCreateReadBackLagKeepsId(t *testing.T) {
	f, ts := newFakeGroupController(t)
	f.set(func(f *fakeGroupController) { f.hideReads = 2 })

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + stateTestGroup,
				Check:  resource.TestCheckResourceAttr("omada_ip_port_group.test", "group_id", "group-1"),
			},
		},
	})
}

// When the read-back never converges, the created group must stay in state
// (tainted) under the POST's id, so the next apply replaces it instead of
// creating a duplicate beside an unmanaged original.
func TestAcc_IPPortGroupCreateReadBackFailsKeepsState(t *testing.T) {
	f, ts := newFakeGroupController(t)
	f.set(func(f *fakeGroupController) { f.hideAll = true })

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + stateTestGroup,
				ExpectError: regexp.MustCompile(`kept in\s+state as tainted`),
			},
			{
				PreConfig: func() { f.set(func(f *fakeGroupController) { f.hideAll = false }) },
				Config:    ts.ProviderConfig + stateTestGroup,
				// The tainted group-1 is deleted and group-2 created.
				Check: resource.TestCheckResourceAttr("omada_ip_port_group.test", "group_id", "group-2"),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, orphan := f.rows["group-1"]; orphan {
		t.Fatal("the group created by the failed apply was orphaned on the controller")
	}
}
