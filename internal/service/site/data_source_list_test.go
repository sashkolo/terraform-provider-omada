package site_test

import (
	"net/http"
	"regexp"
	"terraform-provider-omada/internal/acctest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAcc_SitesDataSource(t *testing.T) {
	ts := acctest.NewTestServer(t)
	mux, providerCfg := ts.Mux, ts.ProviderConfig

	sites := `{
		"errorCode": 0,
		"msg": "",
		"result": {
			"totalRows": 0,
			"currentPage": 0,
			"currentSize": 0,
			"data": [
				{
					"siteId": "test-site-id",
					"name": "test-site-name",
					"tagIds": [],
					"region": "United Kingdom",
					"timeZone": "UTC",
					"scenario": "Home",
					"longitude": 0,
					"latitude": 0,
					"address": "",
					"type": 0,
					"supportES": true,
					"supportL2": true,
					"sitePublicIp": "",
					"primary": true
				}
			]
		}
	}`

	mux.HandleFunc("GET /openapi/v1/{omadacId}/sites", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sites))
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerCfg + `data "omada_sites" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify number of sites returned
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.#", "1"),
					// Verify the first site to ensure all attributes are set
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.site_id", "test-site-id"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.name", "test-site-name"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.region", "United Kingdom"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.scenario", "Home"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.time_zone", "UTC"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.type", "0"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.support_es", "true"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.support_l2", "true"),
				),
			},
		},
	})
}

// TestAcc_SitesDataSourceControllerError is a regression test: an
// error envelope (here an expired token) has no result, and the data source
// dereferenced it and crashed the provider instead of reporting the error.
func TestAcc_SitesDataSourceControllerError(t *testing.T) {
	ts := acctest.NewTestServer(t)

	ts.Mux.HandleFunc("GET /openapi/v1/{omadacId}/sites", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorCode":-44112,"msg":"The access token has expired."}`))
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ts.ProviderConfig + `data "omada_sites" "test" {}`,
				ExpectError: regexp.MustCompile(`error code -44112`),
			},
		},
	})
}
