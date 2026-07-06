# Site ID the AP belongs to. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# The WLAN group the AP should broadcast. An AP belongs to exactly one group, so
# this resource asserts (and drift-detects) which single group each AP is on.
resource "omada_wlan_group" "default" {
  site_id = var.site_id
  name    = "Default"
}

# Bind a specific access point to a WLAN group. Because the relationship is 1:1,
# switching the AP to a different group changes which SSIDs it broadcasts and can
# disrupt clients — apply off-hours. To keep a staged group's SSIDs from
# broadcasting, simply never point an AP at that group.
resource "omada_ap_wlan_group" "living_room" {
  site_id       = var.site_id
  ap_mac        = "A4-2B-B0-11-22-33"
  wlan_group_id = omada_wlan_group.default.wlan_group_id
}
