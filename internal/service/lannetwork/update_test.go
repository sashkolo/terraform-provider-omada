package lannetwork_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// updateTestLanNetwork renders the test network with a given name and VLAN.
func updateTestLanNetwork(name string, vlan int, withPorts bool) string {
	ports := `interface_ids  = ["port-1"]`
	if !withPorts {
		ports = ""
	}
	return fmt.Sprintf(`
resource "omada_lan_network" "test" {
	site_id        = "test-site-id"
	name           = %q
	vlan_id        = %d
	gateway_subnet = "192.168.40.1/24"
	%s

	dhcp_settings = {
		enable       = true
		dhcpns       = "auto"
		ipaddr_start = "192.168.40.20"
		ipaddr_end   = "192.168.40.254"
		leasetime    = 1440
	}
}
`, name, vlan, ports)
}

// An edit must not reset settings the config leaves unset or the resource
// doesn't model (homelab #515). On 0.14.0 igmp_snoop_enable was unknown on
// update and sent as false, and the body carried only modeled fields.
func TestAcc_LanNetworkUpdateKeepsUnsetLiveSettings(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + updateTestLanNetwork("Outdoor", 40, true)},
			{
				// Turned on in the controller UI.
				PreConfig: f.set(func(f *fakeLanController) {
					for _, row := range f.rows {
						row["igmpSnoopEnable"] = true
						row["mldSnoopEnable"] = true
						row["isolation"] = true
						row["dhcpSettingsVO"].(map[string]any)["option66"] = "10.0.0.5"
					}
				}),
				Config: ts.ProviderConfig + updateTestLanNetwork("Outdoor renamed", 40, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "igmp_snoop_enable", "true"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						body := f.lastPatch
						dhcp, _ := body["dhcpSettingsVO"].(map[string]any)
						for key, want := range map[string]any{
							"igmpSnoopEnable":         true,
							"mldSnoopEnable":          true,
							"isolation":               true,
							"dhcpSettingsVO.option66": "10.0.0.5",
						} {
							got := body[key]
							if key == "dhcpSettingsVO.option66" {
								got = dhcp["option66"]
							}
							if got != want {
								return fmt.Errorf("update sent %s=%v, want the live %v", key, got, want)
							}
						}
						return nil
					},
				),
			},
		},
	})
}

// A setting the SDK can't send faithfully (here DHCP guard) must stop the
// update before anything is written, rather than risk resetting it.
func TestAcc_LanNetworkUpdateRefusedWhenUnmodeledSettingOn(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + updateTestLanNetwork("Outdoor", 40, true)},
			{
				PreConfig: f.set(func(f *fakeLanController) {
					for _, row := range f.rows {
						row["dhcpGuard"] = map[string]any{"enable": true}
					}
				}),
				Config:      ts.ProviderConfig + updateTestLanNetwork("Outdoor renamed", 40, true),
				ExpectError: regexp.MustCompile(`DHCP guard turned on`),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.patches != 0 {
		t.Fatalf("the refused update still sent %d PATCH request(s)", f.patches)
	}
}

// Changing the VLAN tag is an in-place update: on 0.14.0 it forced a
// destroy-then-create of a live network and everything referencing it.
func TestAcc_LanNetworkVlanChangeInPlace(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + updateTestLanNetwork("Outdoor", 40, true)},
			{
				Config: ts.ProviderConfig + updateTestLanNetwork("Outdoor", 41, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "network_id", "net-1"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "vlan_id", "41"),
				),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.posts != 1 {
		t.Fatalf("the VLAN change created %d network(s); want the original updated in place", f.posts)
	}
}

// Removing interface_ids from config must not unbind the network's ports.
// On 0.14.0 an unset list was sent as [].
func TestAcc_LanNetworkUnsetInterfaceIdsRefused(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + updateTestLanNetwork("Outdoor", 40, true)},
			{
				Config:      ts.ProviderConfig + updateTestLanNetwork("Outdoor", 40, false),
				ExpectError: regexp.MustCompile(`interface_ids is unset`),
			},
		},
	})

	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.rows {
		if ids, _ := row["interfaceIds"].([]any); len(ids) != 1 {
			t.Fatalf("the network's ports changed to %v", row["interfaceIds"])
		}
	}
}
