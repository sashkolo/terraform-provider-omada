package ipmacbinding_test

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
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const resourceName = "omada_gateway_ip_mac_binding.test"

// fakeBindings models the controller's IP-MAC binding table: a paged grid that
// is the only way to read an entry (the grid rows carry the id, the create
// answer doesn't), a POST that rejects a MAC or IP that is already bound, and
// PATCH/DELETE addressed by binding id. The rows carry the deprecated fields
// the live controller returns, to prove the lenient decode ignores them.
type fakeBindings struct {
	mu       sync.Mutex
	rows     map[string]map[string]any // by id
	hidden   bool                      // grid reads omit every row
	pageSize int                       // rows per grid page (0 = all)
	posts    int
	patches  []map[string]any
	deletes  []string
}

func newFakeBindings(t *testing.T) (*fakeBindings, *acctest.TestServer) {
	f := &fakeBindings{rows: map[string]map[string]any{}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	const base = "/openapi/v1/{omadacId}/sites/{siteId}/ip-mac-binds"

	ts.Mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, row := range f.rows {
			if row["mac"] == body["mac"] || row["ip"] == body["ip"] {
				write(w, map[string]any{"errorCode": -39741, "msg": "The IP or MAC address is already bound."})
				return
			}
		}
		f.posts++
		id := fmt.Sprintf("bind-%d", f.posts)
		body["id"] = id
		body["exportToDhcpReservation"] = false
		body["supportExport"] = false
		if d, ok := body["description"]; ok && d == "" {
			delete(body, "description")
		}
		f.rows[id] = body
		// The live create answer carries no result.
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	ts.Mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		ids := []string{}
		if !f.hidden {
			for id := range f.rows {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		page, size := 1, len(ids)
		if f.pageSize > 0 {
			size = f.pageSize
			_, _ = fmt.Sscan(r.URL.Query().Get("page"), &page)
		}
		data := []any{}
		for i := (page - 1) * size; i < len(ids) && i < page*size; i++ {
			data = append(data, f.rows[ids[i]])
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"totalRows": len(ids), "currentPage": page, "currentSize": size, "data": data,
		}})
	})
	ts.Mux.HandleFunc("PATCH "+base+"/{bindId}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.patches = append(f.patches, body)
		row, ok := f.rows[r.PathValue("bindId")]
		if !ok {
			write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		for k, v := range body {
			row[k] = v
		}
		if d, ok := row["description"]; ok && d == "" {
			delete(row, "description")
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	ts.Mux.HandleFunc("DELETE "+base+"/{bindId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		id := r.PathValue("bindId")
		f.deletes = append(f.deletes, id)
		if _, ok := f.rows[id]; !ok {
			write(w, map[string]any{"errorCode": -1001, "msg": "Invalid request parameters."})
			return
		}
		delete(f.rows, id)
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func (f *fakeBindings) set(fn func(*fakeBindings)) func() {
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		fn(f)
	}
}

func (f *fakeBindings) check(fn func(*fakeBindings) error) resource.TestCheckFunc {
	return func(*terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		return fn(f)
	}
}

// liveRow is an entry made outside Terraform, shaped like a live grid row.
func liveRow(id, mac, ip, desc string) map[string]any {
	row := map[string]any{
		"id": id, "mac": mac, "ip": ip, "status": true,
		"interfaceType": 1, "interfaceId": "mgmt-net-id",
		"exportToDhcpReservation": false, "supportExport": false,
		"enableExportToDhcpReservation": false,
	}
	if desc != "" {
		row["description"] = desc
	}
	return row
}

func bindingConfig(mac, ip, extra string) string {
	return fmt.Sprintf(`
resource "omada_gateway_ip_mac_binding" "test" {
  site_id      = "test-site-id"
  mac          = %q
  ip           = %q
  interface_id = "mgmt-net-id"
%s
}
`, mac, ip, extra)
}

// Create, read back, import, update the description, clear it, and delete a
// binding on a scratch IP: the full lifecycle the live acceptance check runs.
func TestAcc_IpMacBindingLifecycle(t *testing.T) {
	f, ts := newFakeBindings(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.rows) != 0 {
				return fmt.Errorf("%d bindings left after destroy", len(f.rows))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + bindingConfig("00-00-5E-00-53-10", "192.168.1.250", `  description = "scratch"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "binding_id", "bind-1"),
					resource.TestCheckResourceAttr(resourceName, "mac", "00-00-5E-00-53-10"),
					resource.TestCheckResourceAttr(resourceName, "ip", "192.168.1.250"),
					resource.TestCheckResourceAttr(resourceName, "interface_type", "lan"),
					resource.TestCheckResourceAttr(resourceName, "interface_id", "mgmt-net-id"),
					resource.TestCheckResourceAttr(resourceName, "description", "scratch"),
					resource.TestCheckResourceAttr(resourceName, "status", "true"),
					resource.TestCheckResourceAttr(resourceName, "export_to_dhcp_reservation", "false"),
					f.check(func(f *fakeBindings) error {
						row := f.rows["bind-1"]
						if row["interfaceType"] != float64(1) || row["enableExportToDhcpReservation"] != false {
							return fmt.Errorf("create sent %v", row)
						}
						if _, sent := row["exportToDhcpReservation"]; !sent {
							return fmt.Errorf("fake lost its deprecated field")
						}
						return nil
					}),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/00-00-5E-00-53-10",
				ImportStateVerifyIdentifierAttribute: "mac",
			},
			{
				Config: ts.ProviderConfig + bindingConfig("00-00-5E-00-53-10", "192.168.1.250", `  description = "scratch (renamed)"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "binding_id", "bind-1"),
					resource.TestCheckResourceAttr(resourceName, "description", "scratch (renamed)"),
				),
			},
			{
				// Removing the description sends "" so the controller clears it.
				Config: ts.ProviderConfig + bindingConfig("00-00-5E-00-53-10", "192.168.1.250", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceName, "description"),
					f.check(func(f *fakeBindings) error {
						last := f.patches[len(f.patches)-1]
						if d, ok := last["description"]; !ok || d != "" {
							return fmt.Errorf("clearing the description sent %v", last)
						}
						return nil
					}),
				),
			},
		},
	})
}

// An existing entry imported through an import block plans no change, and
// nothing is written: the adoption path for a binding made in the UI.
func TestAcc_IpMacBindingImportBlockIsNoOp(t *testing.T) {
	f, ts := newFakeBindings(t)
	f.rows["live-1"] = liveRow("live-1", "00-00-5E-00-53-19", "192.168.1.19", "desk PC")
	f.rows["live-2"] = liveRow("live-2", "00-00-5E-00-53-05", "192.168.1.5", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:          ts.ProviderConfig + bindingConfig("00-00-5E-00-53-19", "192.168.1.19", `  description = "desk PC"`),
				ResourceName:    resourceName,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   "test-site-id/00-00-5E-00-53-19",
			},
		},
	})
	if f.posts != 0 || len(f.patches) != 0 || len(f.deletes) != 0 {
		t.Fatalf("import wrote: %d POSTs, %d PATCHes, %d DELETEs", f.posts, len(f.patches), len(f.deletes))
	}
}

// After import, the matching configuration plans empty, a lower-case colon MAC
// in the configuration still matches, and drift made in the UI shows as an
// in-place update.
func TestAcc_IpMacBindingImportThenDrift(t *testing.T) {
	f, ts := newFakeBindings(t)
	f.rows["live-1"] = liveRow("live-1", "00-00-5E-00-53-19", "192.168.1.19", "desk PC")
	cfg := ts.ProviderConfig + bindingConfig("00:00:5e:00:53:19", "192.168.1.19", `  description = "desk PC"`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ResourceName:       resourceName,
				ImportState:        true,
				ImportStateId:      "test-site-id/00:00:5e:00:53:19",
				ImportStatePersist: true,
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "binding_id", "live-1"),
					resource.TestCheckResourceAttr(resourceName, "mac", "00:00:5e:00:53:19"),
				),
			},
			{
				// The UI disables the entry and changes the IP: the plan
				// repairs both in place, addressed by id.
				PreConfig: f.set(func(f *fakeBindings) {
					f.rows["live-1"]["status"] = false
					f.rows["live-1"]["ip"] = "192.168.1.99"
				}),
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate)},
				},
				Check: f.check(func(f *fakeBindings) error {
					row := f.rows["live-1"]
					// The colon-lower-case configured MAC is sent canonical.
					if row["status"] != true || row["ip"] != "192.168.1.19" || row["mac"] != "00-00-5E-00-53-19" {
						return fmt.Errorf("drift not repaired: %v", row)
					}
					return nil
				}),
			},
			{
				// A binding deleted in the UI is planned for re-creation.
				PreConfig:          f.set(func(f *fakeBindings) { delete(f.rows, "live-1") }),
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// Create refuses a MAC or IP that another entry binds and names it, instead of
// sending a POST.
func TestAcc_IpMacBindingCreateConflict(t *testing.T) {
	f, ts := newFakeBindings(t)
	f.rows["live-1"] = liveRow("live-1", "00-00-5E-00-53-19", "192.168.1.19", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + bindingConfig("00-00-5e-00-53-19", "192.168.1.250", ""),
				ExpectError: regexp.MustCompile(`MAC 00-00-5e-00-53-19 is already bound by entry live-1`),
			},
			{
				Config:      ts.ProviderConfig + bindingConfig("00-00-5E-00-53-20", "192.168.1.19", ""),
				ExpectError: regexp.MustCompile(`IP 192.168.1.19 is already bound by entry live-1`),
			},
		},
	})
	if f.posts != 0 {
		t.Fatalf("conflicting creates sent %d POSTs, want none", f.posts)
	}
}

// When the read-back after a successful POST never converges, the entry stays
// in state (tainted) under its MAC. The next apply deletes it by resolving the
// id from the MAC, then creates it again.
func TestAcc_IpMacBindingCreateReadBackFailsKeepsState(t *testing.T) {
	f, ts := newFakeBindings(t)
	f.hidden = true
	cfg := ts.ProviderConfig + bindingConfig("00-00-5E-00-53-10", "192.168.1.250", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      cfg,
				ExpectError: regexp.MustCompile(`kept in\s+state as tainted`),
			},
			{
				PreConfig: f.set(func(f *fakeBindings) { f.hidden = false }),
				Config:    cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "binding_id", "bind-2"),
					f.check(func(f *fakeBindings) error {
						if len(f.deletes) != 1 || f.deletes[0] != "bind-1" || len(f.rows) != 1 {
							return fmt.Errorf("want bind-1 deleted and one row left, got deletes %v, %d rows", f.deletes, len(f.rows))
						}
						return nil
					}),
				),
			},
		},
	})
}

// The grid is paged; an entry beyond the first page is still found.
func TestAcc_IpMacBindingPagedGrid(t *testing.T) {
	f, ts := newFakeBindings(t)
	for i := 0; i < 150; i++ {
		id := fmt.Sprintf("a-%03d", i)
		f.rows[id] = liveRow(id, fmt.Sprintf("00-00-5E-00-%02X-%02X", i/256, i%256), fmt.Sprintf("10.0.%d.%d", i/200, i%200+1), "")
	}
	f.pageSize = 100
	f.rows["z-last"] = liveRow("z-last", "00-00-5E-00-53-19", "192.168.1.19", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             ts.ProviderConfig + bindingConfig("00-00-5E-00-53-19", "192.168.1.19", ""),
				ResourceName:       resourceName,
				ImportState:        true,
				ImportStateId:      "test-site-id/00-00-5E-00-53-19",
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if got := states[0].Attributes["binding_id"]; got != "z-last" {
						return fmt.Errorf("binding_id = %q, want z-last", got)
					}
					return nil
				},
			},
		},
	})
}

// A WAN binding round-trips its interface type, and the export flag is sent
// and read back.
func TestAcc_IpMacBindingWanAndExport(t *testing.T) {
	f, ts := newFakeBindings(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + bindingConfig("00-00-5E-00-53-30", "203.0.113.30",
					"  interface_type = \"wan\"\n  export_to_dhcp_reservation = true\n  status = false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "interface_type", "wan"),
					resource.TestCheckResourceAttr(resourceName, "export_to_dhcp_reservation", "true"),
					resource.TestCheckResourceAttr(resourceName, "status", "false"),
					f.check(func(f *fakeBindings) error {
						row := f.rows["bind-1"]
						if row["interfaceType"] != float64(0) || row["enableExportToDhcpReservation"] != true || row["status"] != false {
							return fmt.Errorf("create sent %v", row)
						}
						return nil
					}),
				),
			},
		},
	})
}

// A destroy of an entry already removed in the UI succeeds.
func TestAcc_IpMacBindingDeleteAlreadyGone(t *testing.T) {
	f, ts := newFakeBindings(t)
	cfg := ts.ProviderConfig + bindingConfig("00-00-5E-00-53-10", "192.168.1.250", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: f.set(func(f *fakeBindings) { delete(f.rows, "bind-1") }),
				Config:    cfg,
				Destroy:   true,
			},
		},
	})
}

// Malformed configuration and import ids are rejected before any request.
func TestAcc_IpMacBindingValidation(t *testing.T) {
	_, ts := newFakeBindings(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + bindingConfig("not-a-mac", "192.168.1.250", ""),
				ExpectError: regexp.MustCompile(`Invalid MAC address`),
			},
			{
				Config:      ts.ProviderConfig + bindingConfig("00-00-5E-00-53-10", "192.168.1.250", `  interface_type = "vpn"`),
				ExpectError: regexp.MustCompile(`value must be one of: lan, wan`),
			},
			{
				Config: ts.ProviderConfig + bindingConfig("00-00-5E-00-53-10", "192.168.1.250",
					`  description = "this description is far too long for the controller, which allows 64"`),
				ExpectError: regexp.MustCompile(`length must be between 1 and 64`),
			},
			{
				Config:        ts.ProviderConfig + bindingConfig("00-00-5E-00-53-10", "192.168.1.250", ""),
				ResourceName:  resourceName,
				ImportState:   true,
				ImportStateId: "test-site-id",
				ExpectError:   regexp.MustCompile(`Expected import ID in the form`),
			},
		},
	})
}
