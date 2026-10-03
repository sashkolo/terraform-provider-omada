package lannetwork_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// optionsTestLanNetwork renders a DHCP-serving test network. extra is spliced
// into the resource body (e.g. isolation) and options into dhcp_settings.
func optionsTestLanNetwork(name, extra, options string) string {
	return fmt.Sprintf(`
resource "omada_lan_network" "test" {
	site_id        = "test-site-id"
	name           = %q
	vlan_id        = 60
	gateway_subnet = "192.168.160.1/24"
	interface_ids  = ["port-1"]
	%s

	dhcp_settings = {
		enable       = true
		dhcpns       = "auto"
		ipaddr_start = "192.168.160.20"
		ipaddr_end   = "192.168.160.254"
		leasetime    = 1440
		%s
	}
}
`, name, extra, options)
}

const ntpOption = `options = [
			{ code = 42, type = 0, value = "192.0.2.10,192.0.2.11" },
		]`

// liveOptions sets custom DHCP options on every row, as the controller UI
// would.
func liveOptions(f *fakeLanController) {
	for _, row := range f.rows {
		if dhcp, ok := row["dhcpSettingsVO"].(map[string]any); ok {
			dhcp["options"] = []any{map[string]any{"code": 42, "type": 1, "value": "192.0.2.99"}}
		}
	}
}

// Declared options are sent on create, read back, and plan clean.
func TestAcc_LanNetworkDhcpOptionsRoundTrip(t *testing.T) {
	_, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + optionsTestLanNetwork("IoT", "", ntpOption),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.options.#", "1"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.options.0.code", "42"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.options.0.type", "0"),
					resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.options.0.value", "192.0.2.10,192.0.2.11"),
				),
			},
		},
	})
}

// An option added in the controller UI to a network that declares none shows
// up as drift; on v0.16.1 options weren't read at all.
func TestAcc_LanNetworkDhcpOptionsUIDriftShows(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + optionsTestLanNetwork("IoT", "", "")},
			{
				PreConfig:          f.set(liveOptions),
				Config:             ts.ProviderConfig + optionsTestLanNetwork("IoT", "", ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// Live options that the config doesn't declare would be dropped by an update
// (the SDK omits an empty list), so the update is refused, not sent.
func TestAcc_LanNetworkUpdateRefusesUndeclaredLiveOptions(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + optionsTestLanNetwork("IoT", "", "")},
			{
				PreConfig:   f.set(liveOptions),
				Config:      ts.ProviderConfig + optionsTestLanNetwork("IoT renamed", "", ""),
				ExpectError: regexp.MustCompile(`dhcp_settings\.options\s+doesn't\s+declare`),
			},
		},
	})
}

// Declared options are sent on update, so an edit to another attribute keeps
// them; on v0.16.1 any live option refused every update.
func TestAcc_LanNetworkUpdateSendsDeclaredOptions(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ts.ProviderConfig + optionsTestLanNetwork("IoT", "", ntpOption)},
			{
				Config: ts.ProviderConfig + optionsTestLanNetwork("IoT renamed", "", ntpOption),
				Check: func(*terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					dhcp, _ := f.lastPatch["dhcpSettingsVO"].(map[string]any)
					opts, _ := dhcp["options"].([]any)
					if len(opts) != 1 {
						return fmt.Errorf("update sent %d DHCP options, want the 1 declared", len(opts))
					}
					o, _ := opts[0].(map[string]any)
					if fmt.Sprint(o["code"]) != "42" || o["value"] != "192.0.2.10,192.0.2.11" {
						return fmt.Errorf("update sent option %v, want code 42 with the declared value", o)
					}
					return nil
				},
			},
		},
	})
}

// IPv6 turned on in the controller is reported read-only, so a config can
// assert it stays off.
func TestAcc_LanNetworkIpv6EnabledReported(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + optionsTestLanNetwork("IoT", "", ""),
				Check:  resource.TestCheckResourceAttr("omada_lan_network.test", "ipv6_enabled", "false"),
			},
			{
				PreConfig: f.set(func(f *fakeLanController) {
					for _, row := range f.rows {
						row["lanNetworkIpv6Config"] = map[string]any{"proto": 0, "enable": 1}
					}
				}),
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("omada_lan_network.test", "ipv6_enabled", "true"),
			},
		},
	})
}

// Isolation turned on in the UI against a config that declares it off shows
// up as drift; on v0.16.1 isolation was carried but never read.
func TestAcc_LanNetworkIsolationDriftShows(t *testing.T) {
	f, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + optionsTestLanNetwork("IoT", "isolation = false", ""),
				Check:  resource.TestCheckResourceAttr("omada_lan_network.test", "isolation", "false"),
			},
			{
				PreConfig: f.set(func(f *fakeLanController) {
					for _, row := range f.rows {
						row["isolation"] = true
					}
				}),
				Config:             ts.ProviderConfig + optionsTestLanNetwork("IoT", "isolation = false", ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// An out-of-range option code is rejected before anything is sent.
func TestAcc_LanNetworkDhcpOptionsValidated(t *testing.T) {
	_, ts := newFakeLanController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + optionsTestLanNetwork("IoT", "", `options = [
			{ code = 300, type = 0, value = "x" },
		]`),
				ExpectError: regexp.MustCompile(`code must be within 1-254`),
			},
		},
	})
}

// Options built from an expression that is unknown at validation time (here a
// conditional on an input variable) must validate; on v0.19.0 ValidateConfig
// read them into a plain slice and failed with a value conversion error.
func TestAcc_LanNetworkDhcpOptionsFromExpression(t *testing.T) {
	_, ts := newFakeLanController(t)

	cfg := `
variable "two_entries" {
	type = bool
}
` + optionsTestLanNetwork("IoT", "", `options = var.two_entries ? [
			{ code = 42, type = 1, value = "192.0.2.10" },
			{ code = 42, type = 1, value = "192.0.2.11" },
		] : [
			{ code = 42, type = 1, value = "192.0.2.10,192.0.2.11" },
		]`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:          ts.ProviderConfig + cfg,
				ConfigVariables: config.Variables{"two_entries": config.BoolVariable(true)},
				Check:           resource.TestCheckResourceAttr("omada_lan_network.test", "dhcp_settings.options.#", "2"),
			},
		},
	})
}
