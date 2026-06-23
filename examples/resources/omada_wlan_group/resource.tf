# Site ID to manage the WLAN group in. Provide via a variable or data source so
# the value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# A staging WLAN group that is intentionally not bound to any AP: SSIDs created
# inside it are managed as code without broadcasting, so they can be staged and
# reviewed before being applied to production access points.
resource "omada_wlan_group" "staging" {
  site_id = var.site_id
  name    = "staging"
}

output "staging_wlan_group_id" {
  value = omada_wlan_group.staging.wlan_group_id
}
