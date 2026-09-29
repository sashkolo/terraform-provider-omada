package switchportprofile_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// fakeProfileController models the parts of the controller that state accuracy
// depends on (homelab #514): a paged list that is the only way to read a
// profile, eventually consistent after a write, and a DELETE that fails when
// the profile is already gone.
type fakeProfileController struct {
	mu        sync.Mutex
	rows      map[string]map[string]any
	hideReads int // that many list reads omit every row
	// vanishCode, when set, removes the profile just as the DELETE arrives and
	// answers with this controller error, as when someone deletes it in the UI
	// between Terraform's refresh and its delete.
	vanishCode int
	posts      int
}

func newFakeProfileController(t *testing.T) (*fakeProfileController, *acctest.TestServer) {
	f := &fakeProfileController{rows: map[string]map[string]any{}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	ts.Mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/lan-profiles", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts++
		id := fmt.Sprintf("profile-%d", f.posts)
		body["id"] = id
		f.rows[id] = body
		write(w, map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{"id": id}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/lan-profiles", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data := []any{}
		if f.hideReads > 0 {
			f.hideReads--
		} else {
			for _, row := range f.rows {
				data = append(data, row)
			}
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"totalRows": len(data), "currentPage": 1, "currentSize": 1000, "data": data,
		}})
	})
	ts.Mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/lan-profiles/{profileId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("profileId")
		if f.vanishCode != 0 {
			delete(f.rows, id)
			write(w, map[string]any{"errorCode": f.vanishCode, "msg": "rejected"})
			return
		}
		if _, ok := f.rows[id]; !ok {
			write(w, map[string]any{"errorCode": -33507, "msg": "This profile does not exist."})
			return
		}
		delete(f.rows, id)
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeProfileController) set(fn func(f *fakeProfileController)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

const stateTestProfile = `
resource "omada_switch_port_profile" "test" {
	site_id            = "test-site-id"
	name               = "Outdoor-Untrusted"
	native_network_id  = "net-outdoor"
	tagged_network_ids = []
}
`

// Destroying a profile that is already gone must succeed. The controller
// answers -33507 ("This profile does not exist"); on 0.14.0 the provider
// expected -33517, so the destroy failed.
func TestAcc_SwitchPortProfileDestroyAlreadyGone(t *testing.T) {
	f, ts := newFakeProfileController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestProfile},
			{
				PreConfig: func() { f.set(func(f *fakeProfileController) { f.vanishCode = -33507 }) },
				Config:    ts.ProviderConfig,
			},
		},
	})
}

// Any other controller error on DELETE is still success when a confirmed
// re-list shows the profile absent.
func TestAcc_SwitchPortProfileDestroyAlreadyGoneOtherCode(t *testing.T) {
	f, ts := newFakeProfileController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestProfile},
			{
				PreConfig: func() { f.set(func(f *fakeProfileController) { f.vanishCode = -1001 }) },
				Config:    ts.ProviderConfig,
			},
		},
	})
}

// A read-back that misses the new profile for a moment must not null the id
// the POST returned (the same null-id bug as the group resources).
func TestAcc_SwitchPortProfileCreateReadBackLagKeepsId(t *testing.T) {
	f, ts := newFakeProfileController(t)
	f.set(func(f *fakeProfileController) { f.hideReads = 2 })

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + stateTestProfile,
				Check:  resource.TestCheckResourceAttr("omada_switch_port_profile.test", "profile_id", "profile-1"),
			},
		},
	})
}
