package switchportprofile_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	testSwitchMac   = "00-00-5E-00-53-01"
	accessSwitchMac = "00-00-5E-00-53-02"
)

// serveSwitches adds the site device list, the switch overview and the
// per-port write to the fake. The overview reports each port's profile, and a
// port write moves the port to the profile it names.
func (f *fakeProfileController) serveSwitches(ts *acctest.TestServer, write func(http.ResponseWriter, any)) {
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/devices", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.deviceErr != 0 {
			write(w, map[string]any{"errorCode": f.deviceErr, "msg": "rejected"})
			return
		}
		all := []any{}
		for i := 0; i < f.extraAps; i++ {
			all = append(all, map[string]any{"mac": "00-00-5E-00-54-" + strconv.Itoa(i), "name": "AP", "type": "ap"})
		}
		all = append(all, map[string]any{"mac": "00-00-5E-00-53-10", "name": "Gateway", "type": "gateway"})
		macs := make([]string, 0, len(f.switches))
		for mac := range f.switches {
			macs = append(macs, mac)
		}
		sort.Strings(macs)
		for _, mac := range macs {
			name := "Core switch"
			if mac == accessSwitchMac {
				name = "Access switch"
			}
			all = append(all, map[string]any{"mac": mac, "name": name, "type": "switch"})
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		lo, hi := min((page-1)*size, len(all)), min(page*size, len(all))
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{
			"totalRows": len(all), "currentPage": page, "currentSize": size, "data": all[lo:hi],
		}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/switches/{switchMac}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		ports, ok := f.switches[r.PathValue("switchMac")]
		if !ok {
			write(w, map[string]any{"errorCode": -39701, "msg": "The device does not exist."})
			return
		}
		list := []any{}
		for port, profile := range ports {
			name := "All"
			if row, ok := f.rows[profile]; ok {
				name, _ = row["name"].(string)
			}
			list = append(list, map[string]any{
				"port": port, "name": "", "profileId": profile, "profileName": name,
				"profileOverrideEnable": false, "poeMode": 2, "status": 1, "lagPort": false,
			})
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{"portList": list}})
	})
	ts.Mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/switches/{switchMac}/ports/{port}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProfileId string `json:"profileId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		port, _ := strconv.Atoi(r.PathValue("port"))
		f.mu.Lock()
		defer f.mu.Unlock()
		f.switches[r.PathValue("switchMac")][int32(port)] = body.ProfileId
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
}

func (f *fakeProfileController) profileCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

// Deleting a profile that a switch port still uses must be refused before the
// DELETE is sent, and must name the port. Once the port is moved, the destroy
// goes through.
func TestAcc_SwitchPortProfileDeleteRefusedWhileInUse(t *testing.T) {
	f, ts := newFakeProfileController(t)
	f.set(func(f *fakeProfileController) {
		f.switches[accessSwitchMac] = map[int32]string{7: "profile-all"}
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestProfile},
			{
				// A port is moved onto the profile outside Terraform.
				PreConfig: func() {
					f.set(func(f *fakeProfileController) { f.switches[accessSwitchMac][8] = "profile-1" })
				},
				Config:      ts.ProviderConfig,
				ExpectError: regexp.MustCompile(`(?s)Profile "Untrusted" is still used by 1 switch\s+port\(s\):\s+switch\s+"Access\s+switch"\s+\(00-00-5E-00-53-02\)\s+port\s+8`),
			},
			{
				PreConfig: func() {
					if n := f.profileCount(); n != 1 {
						t.Fatalf("the refused destroy deleted the profile: %d profiles left, want 1", n)
					}
					f.set(func(f *fakeProfileController) { f.switches[accessSwitchMac][8] = "profile-all" })
				},
				Config: ts.ProviderConfig,
				Check: func(_ *terraform.State) error {
					if n := f.profileCount(); n != 0 {
						return fmt.Errorf("%d profiles left after the destroy, want 0", n)
					}
					return nil
				},
			},
		},
	})
}

// A switch on a later page of the device list is checked too.
func TestAcc_SwitchPortProfileDeleteChecksEveryPage(t *testing.T) {
	f, ts := newFakeProfileController(t)
	f.set(func(f *fakeProfileController) {
		f.extraAps = 120
		f.switches[accessSwitchMac] = map[int32]string{}
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestProfile},
			{
				PreConfig: func() {
					f.set(func(f *fakeProfileController) { f.switches[accessSwitchMac][3] = "profile-1" })
				},
				Config:      ts.ProviderConfig,
				ExpectError: regexp.MustCompile(`(?s)still used by 1 switch\s+port\(s\).*port\s+3`),
			},
			{
				PreConfig: func() {
					f.set(func(f *fakeProfileController) { delete(f.switches[accessSwitchMac], 3) })
				},
				Config: ts.ProviderConfig,
			},
		},
	})
}

// When the switches can't be read, the delete is refused: a failed lookup is
// no evidence that the profile is unused.
func TestAcc_SwitchPortProfileDeleteFailsClosed(t *testing.T) {
	f, ts := newFakeProfileController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + stateTestProfile},
			{
				PreConfig:   func() { f.set(func(f *fakeProfileController) { f.deviceErr = -1 }) },
				Config:      ts.ProviderConfig,
				ExpectError: regexp.MustCompile(`listing the site's switches`),
			},
			{
				PreConfig: func() {
					if n := f.profileCount(); n != 1 {
						t.Fatalf("the refused destroy deleted the profile: %d profiles left, want 1", n)
					}
					f.set(func(f *fakeProfileController) { f.deviceErr = 0 })
				},
				Config: ts.ProviderConfig,
			},
		},
	})
}

// moveConfigs returns a profile and a port binding for the move tests. With
// cbd, the profiles carry create_before_destroy.
func moveConfigs(cbd bool) (func(string) string, func(string) string) {
	lifecycle := ""
	if cbd {
		lifecycle = "\n\tlifecycle {\n\t\tcreate_before_destroy = true\n\t}"
	}
	profile := func(name string) string {
		return `
resource "omada_switch_port_profile" "` + name + `" {
	site_id            = "test-site-id"
	name               = "` + name + `"
	native_network_id  = "net-` + name + `"
	tagged_network_ids = []
	port_isolation_enable  = false
	lldp_med_enable        = true
	loopback_detect_enable = true
	spanning_tree_enable   = true` + lifecycle + `
}
`
	}
	port := func(profile string) string {
		return `
resource "omada_switch_port" "p8" {
	site_id    = "test-site-id"
	switch_mac = "` + testSwitchMac + `"
	port       = 8
	profile_id = omada_switch_port_profile.` + profile + `.profile_id
}
`
	}
	return profile, port
}

// Moving the port to another profile and deleting the old profile in one
// apply is refused by default: Terraform destroys the old profile before it
// updates the port, so the port still uses it at that point. Nothing is
// deleted, and a second apply after the move goes through.
func TestAcc_SwitchPortProfileMovePortAndDeleteRefused(t *testing.T) {
	f, ts := newFakeProfileController(t)
	profile, port := moveConfigs(false)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + profile("old") + profile("new") + port("old")},
			{
				Config:      ts.ProviderConfig + profile("new") + port("new"),
				ExpectError: regexp.MustCompile(`(?s)Profile "old" is still used.*create_before_destroy`),
			},
			{
				// Move first, then delete.
				PreConfig: func() {
					if n := f.profileCount(); n != 2 {
						t.Fatalf("%d profiles after the refused apply, want 2", n)
					}
				},
				Config: ts.ProviderConfig + profile("old") + profile("new") + port("new"),
			},
			{Config: ts.ProviderConfig + profile("new") + port("new")},
			{
				// Free "new" for the post-test destroy.
				PreConfig: func() {
					f.set(func(f *fakeProfileController) { f.switches[testSwitchMac][8] = "profile-all" })
				},
				Config: ts.ProviderConfig + profile("new"),
			},
		},
	})
}

// With create_before_destroy on the profiles, the port is updated before the
// old profile is destroyed, so the move and the delete fit in one apply.
func TestAcc_SwitchPortProfileMovePortAndDeleteInOneApply(t *testing.T) {
	f, ts := newFakeProfileController(t)
	profile, port := moveConfigs(true)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + profile("old") + profile("new") + port("old")},
			{
				Config: ts.ProviderConfig + profile("new") + port("new"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("omada_switch_port.p8", "profile_id", "omada_switch_port_profile.new", "profile_id"),
					func(_ *terraform.State) error {
						if n := f.profileCount(); n != 1 {
							return fmt.Errorf("%d profiles left, want only the new one", n)
						}
						return nil
					},
				),
			},
			{
				// Free "new" for the post-test destroy.
				PreConfig: func() {
					f.set(func(f *fakeProfileController) { f.switches[testSwitchMac][8] = "profile-all" })
				},
				Config: ts.ProviderConfig + profile("new"),
			},
		},
	})
}
