package ipportgroup_test

import (
	"encoding/json"
	"net/http"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAcc_IPPortGroupResource exercises full CRUD + import for the
// omada_ip_port_group resource against an httptest stand-in for the Omada Open
// API. It models a camera service-ports group: the outdoor camera subnet scoped
// to the ONVIF/RTSP/HTTP(S) ports, port_type 0 (port-list mode).
func TestAcc_IPPortGroupResource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	listRow := map[string]any{
		"groupId":     "test-group-id",
		"name":        "camera-service-ports",
		"description": "Camera ONVIF/RTSP/HTTP ports",
		"type":        int32(1),
		"portType":    int32(0),
		"ipList": []any{
			map[string]any{"ip": "192.168.30.0", "mask": int32(24)},
		},
		"portList": []any{"80", "443", "554", "2020"},
	}

	listResponse := func() string {
		b, _ := json.Marshal(map[string]any{
			"errorCode": 0,
			"msg":       "",
			"result":    []any{listRow},
		})
		return string(b)
	}

	const emptyResponse = `{ "errorCode": 0, "msg": "" }`

	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success.","result":{"id":"test-group-id"}}`)
	})

	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, listResponse())
	})

	mux.HandleFunc("PATCH /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}/{groupId}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name != "" {
			listRow["name"] = req.Name
		}
		writeJSON(w, emptyResponse)
	})

	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}/{groupId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, emptyResponse)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + `
				resource "omada_ip_port_group" "test" {
					site_id     = "test-site-id"
					name        = "camera-service-ports"
					description = "Camera ONVIF/RTSP/HTTP ports"
					ip_list = [
						{ ip = "192.168.30.0", mask = 24 },
					]
					port_list = ["80", "443", "554", "2020"]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "group_id", "test-group-id"),
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "name", "camera-service-ports"),
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "port_type", "0"),
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "ip_list.#", "1"),
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "ip_list.0.ip", "192.168.30.0"),
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "ip_list.0.mask", "24"),
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "port_list.#", "4"),
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "port_list.2", "554"),
				),
			},
			{
				ResourceName:                         "omada_ip_port_group.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "test-site-id/test-group-id",
				ImportStateVerifyIdentifierAttribute: "group_id",
			},
			{
				Config: ts.ProviderConfig + `
				resource "omada_ip_port_group" "test" {
					site_id     = "test-site-id"
					name        = "camera-service-ports-2"
					description = "Camera ONVIF/RTSP/HTTP ports"
					ip_list = [
						{ ip = "192.168.30.0", mask = 24 },
					]
					port_list = ["80", "443", "554", "2020"]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "name", "camera-service-ports-2"),
				),
			},
		},
	})
}

// TestAcc_IPPortGroupResource_CreateWithoutId covers the create path when the
// controller omits the id in the create response: the resource must recover
// group_id via a name-based list lookup. group_id is Computed (Unknown at
// create), so the fallback guard must treat Unknown as "not yet known".
func TestAcc_IPPortGroupResource_CreateWithoutId(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux := ts.Mux

	listRow := map[string]any{
		"groupId":  "recovered-group-id",
		"name":     "camera-service-ports",
		"type":     int32(1),
		"portType": int32(0),
		"ipList": []any{
			map[string]any{"ip": "192.168.30.0", "mask": int32(24)},
		},
		"portList": []any{"554"},
	}
	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}

	mux.HandleFunc("POST /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errorCode":0,"msg":"Success."}`)
	})
	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}", func(w http.ResponseWriter, _ *http.Request) {
		b, _ := json.Marshal(map[string]any{"errorCode": 0, "msg": "", "result": []any{listRow}})
		writeJSON(w, string(b))
	})
	mux.HandleFunc("DELETE /openapi/v1/{omadacId}/sites/{siteId}/profiles/groups/{groupType}/{groupId}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{ "errorCode": 0, "msg": "" }`)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ts.ProviderConfig + `
				resource "omada_ip_port_group" "test" {
					site_id = "test-site-id"
					name    = "camera-service-ports"
					ip_list = [
						{ ip = "192.168.30.0", mask = 24 },
					]
					port_list = ["554"]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ip_port_group.test", "group_id", "recovered-group-id"),
				),
			},
		},
	})
}
