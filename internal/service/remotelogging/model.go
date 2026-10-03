package remotelogging

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// remoteLoggingClient is the SDK handle shared by the resource. It is
// populated from the provider Meta during Configure.
type remoteLoggingClient struct {
	client   *omada.APIClient
	omadacId string
}

// remoteLoggingResourceModel maps the omada_remote_logging_setting resource
// schema: the site's remote syslog target. This is a singleton per site (the
// object always exists); the resource imports as `<site_id>`, Create/Update
// PATCH it, Read GETs it, and Delete is a no-op.
type remoteLoggingResourceModel struct {
	SiteId        types.String `tfsdk:"site_id"`
	Enable        types.Bool   `tfsdk:"enable"`
	Host          types.String `tfsdk:"host"`
	Port          types.Int32  `tfsdk:"port"`
	MoreClientLog types.Bool   `tfsdk:"more_client_log"`
}
