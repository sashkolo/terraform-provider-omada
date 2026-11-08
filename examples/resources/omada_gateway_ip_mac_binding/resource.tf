# Site ID that owns the binding. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# The LAN network the bound address belongs to.
resource "omada_lan_network" "office" {
  site_id        = var.site_id
  name           = "Office"
  vlan_id        = 110
  gateway_subnet = "192.168.110.1/24"
  interface_ids  = [] # gateway LAN port ids

  dhcp_settings = {
    enable       = true
    ipaddr_start = "192.168.110.100"
    ipaddr_end   = "192.168.110.254"
  }
}

# Hand the desk PC a fixed address...
resource "omada_dhcp_reservation" "desk_pc" {
  site_id = var.site_id
  mac     = "00-00-5E-00-53-19"
  ip      = "192.168.110.19"
  net_id  = omada_lan_network.office.network_id
}

# ...and bind the pair on the gateway, so the gateway keeps a static ARP entry
# for it. A directed wake-on-LAN packet from the gateway then still reaches the
# PC after it has slept long enough for its dynamic ARP entry to expire.
resource "omada_gateway_ip_mac_binding" "desk_pc" {
  site_id      = var.site_id
  mac          = omada_dhcp_reservation.desk_pc.mac
  ip           = omada_dhcp_reservation.desk_pc.ip
  interface_id = omada_lan_network.office.network_id
  description  = "desk PC (wake path)"
}
