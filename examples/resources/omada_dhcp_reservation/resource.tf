# Site ID that owns the reservation. Provide it via a variable or data source so
# the value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# The managed LAN network the reserved address belongs to. A reservation binds a
# MAC to a fixed IP within this network's subnet.
resource "omada_lan_network" "untrusted" {
  site_id        = var.site_id
  name           = "Untrusted"
  vlan_id        = 100
  gateway_subnet = "192.168.100.1/24"
  interface_ids  = [] # gateway LAN port ids

  dhcp_settings = {
    enable       = true
    ipaddr_start = "192.168.100.100" # keep the dynamic pool clear of reservations
    ipaddr_end   = "192.168.100.254"
  }
}

# A DHCP fixed-IP reservation: hand the Porch camera camera a stable address
# (outside the dynamic pool above) so ONVIF/RTSP/Home Assistant integrations can
# rely on it. The reservation is keyed by MAC on the controller.
resource "omada_dhcp_reservation" "porch_camera" {
  site_id     = var.site_id
  mac         = "00-00-5E-00-53-21"
  ip          = "192.168.100.21"
  net_id      = omada_lan_network.untrusted.network_id
  description = "Porch camera"
}

output "porch_camera_reservation_id" {
  value = omada_dhcp_reservation.porch_camera.reservation_id
}
