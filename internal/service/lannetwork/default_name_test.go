package lannetwork_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// defaultNameLanNetwork renders the "Main LAN" network with a given DHCP pool
// start.
func defaultNameLanNetwork(poolStart string) string {
	return fmt.Sprintf(`
resource "omada_lan_network" "test" {
	site_id        = "test-site-id"
	name           = "Main LAN"
	vlan_id        = 1
	gateway_subnet = "192.168.120.1/24"
	interface_ids  = ["port-1"]

	dhcp_settings = {
		enable       = true
		dhcpns       = "auto"
		ipaddr_start = %q
		ipaddr_end   = "192.168.120.254"
		leasetime    = 1440
	}
}
`, poolStart)
}

// markPrimary turns the test network into the site's default network.
func (f *fakeLanController) markPrimary() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.rows {
		row["primary"] = true
	}
}

// checkStoredName asserts the name the controller stores, and that the last
// update sent it without the display suffix.
func (f *fakeLanController) checkStoredName(want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, row := range f.rows {
			if got := row["name"]; got != want {
				return fmt.Errorf("controller stores name %q, want %q", got, want)
			}
		}
		if got := f.lastPatch["name"]; got != want {
			return fmt.Errorf("update sent name %q, want %q", got, want)
		}
		return nil
	}
}

// The list reports the default network as "<name>(Default)". On 0.17.0 that
// suffix was read into state: the plan showed a rename, and any update of the
// default network sent the suffixed name, which the controller stored, and
// failed the apply with an inconsistent result.
func TestAcc_LanNetworkDefaultNetworkName(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + defaultNameLanNetwork("192.168.120.20")},
			{
				// Read as the default network: no drift.
				PreConfig: f.markPrimary,
				Config:    ts.ProviderConfig + defaultNameLanNetwork("192.168.120.20"),
				PlanOnly:  true,
			},
			{
				// Updating it keeps the stored name and reads back consistently.
				Config: ts.ProviderConfig + defaultNameLanNetwork("192.168.120.100"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "name", "Main LAN"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.ipaddr_start", "192.168.120.100"),
					f.checkStoredName("Main LAN"),
				),
			},
		},
	})
}

// A default network that 0.17.0 already renamed (the stored name now ends in
// the suffix) shows the rename as drift, and the apply restores the name.
func TestAcc_LanNetworkDefaultNetworkNameRepair(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + defaultNameLanNetwork("192.168.120.20")},
			{
				PreConfig: f.set(func(f *fakeLanController) {
					for _, row := range f.rows {
						row["primary"] = true
						row["name"] = "Main LAN(Default)"
					}
				}),
				Config:             ts.ProviderConfig + defaultNameLanNetwork("192.168.120.20"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: ts.ProviderConfig + defaultNameLanNetwork("192.168.120.20"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "name", "Main LAN"),
					f.checkStoredName("Main LAN"),
				),
			},
		},
	})
}
