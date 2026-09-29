package dhcpreservation_test

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

// fakeReservationController models the parts of the controller that state
// accuracy depends on: a MAC-keyed grid that is the only way to
// read a reservation, a POST that rejects a MAC that is already reserved, and a
// switch to hide every row from the grid.
type fakeReservationController struct {
	mu     sync.Mutex
	rows   map[string]map[string]any // by MAC
	hidden bool                      // grid reads omit every row (a read-back that never converges)
	posts  int
}

func newFakeReservationController(t *testing.T) (*fakeReservationController, *acctest.TestServer) {
	f := &fakeReservationController{rows: map[string]map[string]any{}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	ts.Mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		mac, _ := body["mac"].(string)
		if _, dup := f.rows[mac]; dup {
			write(w, map[string]any{"errorCode": -33215, "msg": "The MAC address already has a reservation."})
			return
		}
		f.posts++
		id := fmt.Sprintf("res-%d", f.posts)
		body["id"] = id
		body["netName"] = "Personal"
		f.rows[mac] = body
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"id": id}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data := []any{}
		if !f.hidden {
			for _, row := range f.rows {
				data = append(data, row)
			}
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"totalRows": len(data), "currentPage": 1, "currentSize": len(data), "data": data,
		}})
	})
	ts.Mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/setting/service/dhcp/{mac}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		mac := r.PathValue("mac")
		if _, ok := f.rows[mac]; !ok {
			write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		delete(f.rows, mac)
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeReservationController) setHidden(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hidden = v
}

const stateTestReservation = `
resource "omada_dhcp_reservation" "test" {
	site_id = "test-site-id"
	mac     = "00-00-5E-00-53-31"
	ip      = "192.168.110.13"
	net_id  = "personal-net-id"
}
`

// When the read-back after a successful POST never converges, the created
// reservation must stay in state (tainted) under its MAC, so the next plan
// replaces it. Forgetting it made the next apply POST the same MAC again,
// which the controller rejects as a duplicate.
func TestAcc_DhcpReservationCreateReadBackFailsKeepsState(t *testing.T) {
	f, ts := newFakeReservationController(t)
	f.setHidden(true)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + stateTestReservation,
				ExpectError: regexp.MustCompile(`kept in\s+state as tainted`),
			},
			{
				PreConfig: func() { f.setHidden(false) },
				Config:    ts.ProviderConfig + stateTestReservation,
				// The tainted reservation is deleted by MAC and created again.
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "reservation_id", "res-2"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.posts != 2 || len(f.rows) != 1 {
							return fmt.Errorf("want one reservation from two creates, got %d rows from %d creates", len(f.rows), f.posts)
						}
						return nil
					},
				),
			},
		},
	})
}
