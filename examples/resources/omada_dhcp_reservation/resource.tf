# Site ID that owns the reservation. Provide it via a variable or data source so
# the value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# The managed LAN network the reserved address belongs to. A reservation binds a
# MAC to a fixed IP within this network's subnet.
resource "omada_lan_network" "outdoor" {
  site_id        = var.site_id
  name           = "Outdoor-Untrusted"
  vlan_id        = 30
  gateway_subnet = "192.168.30.1/24"
  interface_ids  = [] # gateway LAN port ids

  dhcp_settings = {
    enable       = true
    ipaddr_start = "192.168.30.100" # keep the dynamic pool clear of reservations
    ipaddr_end   = "192.168.30.254"
  }
}

# A DHCP fixed-IP reservation: hand the VIGI South camera a stable address
# (outside the dynamic pool above) so ONVIF/RTSP/Home Assistant integrations can
# rely on it. The reservation is keyed by MAC on the controller.
resource "omada_dhcp_reservation" "vigi_south" {
  site_id     = var.site_id
  mac         = "48-22-54-C3-4C-DE"
  ip          = "192.168.30.21"
  net_id      = omada_lan_network.outdoor.network_id
  description = "VIGI South"
}

output "vigi_south_reservation_id" {
  value = omada_dhcp_reservation.vigi_south.reservation_id
}
