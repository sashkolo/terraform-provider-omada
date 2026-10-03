variable "site_id" {
  type = string
}

# Fail the plan if someone switches the gateway ACL mode in the controller UI:
# the ACLs managed with omada_acl only apply in profile mode (0). A `check`
# block would only warn; a postcondition stops the plan.
data "omada_gateway_acl_config_mode" "current" {
  site_id = var.site_id

  lifecycle {
    postcondition {
      condition     = self.mode == 0
      error_message = "The gateway ACL mode is no longer profile-based; the managed ACLs may not be enforced."
    }
  }
}
