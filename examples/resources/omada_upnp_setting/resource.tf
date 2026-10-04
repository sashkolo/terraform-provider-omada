# Site ID that owns the setting. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# UPnP is a singleton per site: import the live setting first
# (`tofu import omada_upnp_setting.default <site_id>`, or an `import` block) so
# the baseline plan is a no-op before managing it. Destroy only removes it from
# state and leaves the controller's setting as it is; set enable = false and
# apply to turn UPnP off.
resource "omada_upnp_setting" "default" {
  site_id = var.site_id
  enable  = false

  # Declaring the empty selection makes a selection made in the controller UI
  # show up as drift.
  network_ids  = []
  wan_port_ids = []

  # To turn UPnP on, name the LAN networks and WAN ports it serves:
  # enable       = true
  # network_ids  = [omada_lan_network.home.network_id]
  # wan_port_ids = ["<wan port id>"]
}
