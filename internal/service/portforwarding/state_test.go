package portforwarding_test

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

// fakePortForwardingController models the parts of the controller that state
// accuracy depends on: a paged list that is the only way to read
// a rule, and switches to hide rules from it or to ignore the page parameter.
type fakePortForwardingController struct {
	mu     sync.Mutex
	rows   map[string]map[string]any
	hidden bool // list reads omit every row (a read-back that never converges)
	// endlessPages answers every list read with a full page and no totalRows,
	// as a controller that ignores `page` would.
	endlessPages bool
	listReads    int
	posts        int
}

func newFakePortForwardingController(t *testing.T) (*fakePortForwardingController, *acctest.TestServer) {
	f := &fakePortForwardingController{rows: map[string]map[string]any{}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	ts.Mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/nat/port-forwardings", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts++
		id := fmt.Sprintf("pf-%d", f.posts)
		body["id"] = id
		f.rows[id] = body
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"id": id}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/nat/port-forwardings", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.listReads++
		if f.endlessPages {
			if f.listReads > 500 {
				// Stop a provider that never gives up, so the test fails
				// instead of hanging.
				write(w, map[string]any{"errorCode": -1, "msg": "fake controller: too many list reads"})
				return
			}
			data := make([]any, 1000)
			for i := range data {
				data[i] = map[string]any{"id": fmt.Sprintf("other-%d", i), "name": "other", "forwardIp": "192.168.1.9"}
			}
			write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{"data": data}})
			return
		}
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
	ts.Mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/nat/port-forwardings/{portForwardingId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.rows, r.PathValue("portForwardingId"))
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakePortForwardingController) set(fn func(f *fakePortForwardingController)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

const stateTestPortForwarding = `
resource "omada_port_forwarding" "test" {
  site_id          = "test-site-id"
  name             = "WireGuard"
  status           = true
  external_port    = "51820"
  forward_ip       = "192.168.1.5"
  forward_port     = "51820"
  protocol         = 2
  wan_port_ids     = ["wan-primary"]
  source_addresses = []
}
`

// When the read-back after a successful POST never converges, the created rule
// must stay in state (tainted), so the next apply replaces it. On 0.14.0 Create
// returned the error with no state and the open port forward was orphaned.
func TestAcc_PortForwardingCreateReadBackFailsKeepsState(t *testing.T) {
	f, ts := newFakePortForwardingController(t)
	f.set(func(f *fakePortForwardingController) { f.hidden = true })

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + stateTestPortForwarding,
				ExpectError: regexp.MustCompile(`kept\s+in\s+state as tainted`),
			},
			{
				PreConfig: func() { f.set(func(f *fakePortForwardingController) { f.hidden = false }) },
				Config:    ts.ProviderConfig + stateTestPortForwarding,
				// The tainted pf-1 is deleted and pf-2 created.
				Check: resource.TestCheckResourceAttr("omada_port_forwarding.test", "port_forwarding_id", "pf-2"),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, orphan := f.rows["pf-1"]; orphan {
		t.Fatal("the rule created by the failed apply was orphaned on the controller")
	}
}

// A controller that ignores the page parameter and returns full pages without
// totalRows must end in an error, not an endless paging loop.
func TestAcc_PortForwardingListPageCap(t *testing.T) {
	f, ts := newFakePortForwardingController(t)
	f.set(func(f *fakePortForwardingController) { f.endlessPages = true })

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        ts.ProviderConfig + stateTestPortForwarding,
				ResourceName:  "omada_port_forwarding.test",
				ImportState:   true,
				ImportStateId: "test-site-id/pf-missing",
				ExpectError:   regexp.MustCompile(`did not end within 100 pages`),
			},
		},
	})
}
