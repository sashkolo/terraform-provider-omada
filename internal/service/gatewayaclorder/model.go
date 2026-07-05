package gatewayaclorder

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// dragSortTypeGateway is the device-type discriminator for the site-global ACL
// reorder call (DragSortIndexOpenapiVO.Type is "gateway" | "switch" | "eap").
// This resource manages gateway (OSG) ACL order only.
const dragSortTypeGateway = "gateway"

// gatewayAclOrderClient is the SDK handle shared by the resource. It is
// populated from the provider Meta during Configure.
type gatewayAclOrderClient struct {
	client   *omada.APIClient
	omadacId string
}

// gatewayAclOrderResourceModel maps the omada_gateway_acl_order resource schema.
// It is a per-site singleton that owns the *relative order* of every gateway
// (OSG) ACL on the site. Omada evaluates gateway ACLs top-down, and reordering
// is a site-global, per-device-type operation (POST /acls/modifyIndex with the
// full {aclId: index} map and type "gateway"), so a single dedicated resource
// owns it rather than having individual omada_acl resources fight for absolute
// indexes.
//
// ordered_acl_ids must be exhaustive: it lists every gateway ACL id on the site,
// highest priority (evaluated first) first. Read reflects the live order back
// into state, so an out-of-band reorder or a newly-created/-deleted ACL surfaces
// as a plan diff rather than being silently clobbered.
type gatewayAclOrderResourceModel struct {
	SiteId        types.String   `tfsdk:"site_id"`
	OrderedAclIds []types.String `tfsdk:"ordered_acl_ids"`
}
