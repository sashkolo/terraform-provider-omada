# Site ID that owns the gateway ACL order. Provide it via a variable or data
# source so the value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# Deterministic, site-global gateway (OSG) ACL evaluation order. There is one of
# these per site; it must list EVERY gateway ACL id (a missing or extra id is
# rejected so an out-of-band rule is surfaced rather than silently reordered).
#
# Order is highest priority (evaluated first) first. This is how you express
# allow-before-deny: put the allow rule above the broad denies so it takes
# effect. Reference omada_acl.<name>.acl_id so ordering follows the managed rules.
resource "omada_gateway_acl_order" "home" {
  site_id = var.site_id

  ordered_acl_ids = [
    omada_acl.camera_allow.acl_id, # allow, evaluated first
    omada_acl.untrusted_deny_management.acl_id,
    omada_acl.untrusted_deny_personal.acl_id,
    omada_acl.untrusted_deny_public.acl_id,
    omada_acl.untrusted_deny_servers.acl_id,
  ]
}

# Import the existing order for a site with:
#   terraform import omada_gateway_acl_order.home <site_id>
