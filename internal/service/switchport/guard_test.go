package switchport_test

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

// fakeSwitch models a switch as a live 6.2 controller reports it: uplink and AP ports on the "All" trunk
// profile (type 0), access ports on single-VLAN profiles (type 2). Port 3 is a
// LAG member here to exercise that guard.
type fakeSwitch struct {
	mu      sync.Mutex
	ports   map[int32]map[string]any
	patches int
}

func newFakeSwitch(t *testing.T) (*fakeSwitch, *acctest.TestServer) {
	f := &fakeSwitch{ports: map[int32]map[string]any{
		1: {"port": 1, "name": "Port1", "profileId": "prof-all", "profileName": "All", "lagPort": false, "status": 1, "poeMode": 1},
		3: {"port": 3, "name": "Port3", "profileId": "prof-all", "profileName": "All", "lagPort": true, "status": 1, "poeMode": 1},
		8: {"port": 8, "name": "Port8", "profileId": "prof-personal", "profileName": "Personal", "lagPort": false, "status": 1, "poeMode": 1},
	}}
	ts := acctest.NewTestServer(t)

	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/switches/{switchMac}", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		list := []any{}
		for _, p := range f.ports {
			list = append(list, p)
		}
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{"portList": list}})
	})
	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/lan-profiles", func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"errorCode": 0, "msg": "", "result": map[string]any{"data": []any{
			map[string]any{"id": "prof-all", "name": "All", "type": 0, "tagNetworkIds": []string{"n1", "n2", "n3", "n4"}},
			map[string]any{"id": "prof-personal", "name": "Personal", "type": 2, "tagNetworkIds": []string{}},
			map[string]any{"id": "prof-untrusted", "name": "Untrusted", "type": 2, "tagNetworkIds": []string{}},
		}}})
	})
	ts.Mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/switches/{switchMac}/ports/{port}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		var port int32
		_, _ = fmt.Sscan(r.PathValue("port"), &port)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.patches++
		if row, ok := f.ports[port]; ok {
			row["profileId"] = body["profileId"]
			if body["profileId"] == "prof-untrusted" {
				row["profileName"] = "Untrusted"
			}
		}
		write(w, map[string]any{"errorCode": 0, "msg": "Success."})
	})
	return f, ts
}

func portConfig(ts *acctest.TestServer, port int, extra string) string {
	return ts.ProviderConfig + fmt.Sprintf(`
resource "omada_switch_port" "test" {
	site_id    = "test-site-id"
	switch_mac = "00-00-5E-00-53-01"
	port       = %d
	profile_id = "prof-untrusted"
	%s
}
`, port, extra)
}

func (f *fakeSwitch) assertNoWrites(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.patches != 0 {
		t.Fatalf("the refused write still sent %d PATCH request(s)", f.patches)
	}
}

// A typo in `port` must fail before anything is written.
func TestAcc_SwitchPortGuardUnknownPort(t *testing.T) {
	f, ts := newFakeSwitch(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      portConfig(ts, 99, ""),
			ExpectError: regexp.MustCompile(`has no port 99`),
		}},
	})
	f.assertNoWrites(t)
}

// Moving an uplink/AP trunk port to an access profile is refused: on 0.14.0 it
// was applied, cutting off whatever was behind the port.
func TestAcc_SwitchPortGuardTrunkPort(t *testing.T) {
	f, ts := newFakeSwitch(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      portConfig(ts, 1, ""),
			ExpectError: regexp.MustCompile(`trunk profile "All"`),
		}},
	})
	f.assertNoWrites(t)
}

// With allow_trunk_reassign the operator can still make that move deliberately.
func TestAcc_SwitchPortGuardTrunkPortAllowed(t *testing.T) {
	_, ts := newFakeSwitch(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: portConfig(ts, 1, "allow_trunk_reassign = true"),
			Check:  resource.TestCheckResourceAttr("omada_switch_port.test", "profile_id", "prof-untrusted"),
		}},
	})
}

// A LAG member is refused: the write's "switching" operation takes it out of
// its LAG.
func TestAcc_SwitchPortGuardLagMember(t *testing.T) {
	f, ts := newFakeSwitch(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      portConfig(ts, 3, "allow_trunk_reassign = true"),
			ExpectError: regexp.MustCompile(`link-aggregation member`),
		}},
	})
	f.assertNoWrites(t)
}

// An access port moves freely, such as port 8 here.
func TestAcc_SwitchPortGuardAccessPort(t *testing.T) {
	_, ts := newFakeSwitch(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: portConfig(ts, 8, ""),
			Check:  resource.TestCheckResourceAttr("omada_switch_port.test", "profile_id", "prof-untrusted"),
		}},
	})
}
