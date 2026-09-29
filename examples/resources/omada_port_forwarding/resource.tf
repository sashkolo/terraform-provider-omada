# WireGuard UDP port forwarding. Keep the internal endpoint on a stable address
# (for example, an Omada DHCP reservation) and select the intended WAN port
# explicitly. CGNAT links cannot accept unsolicited inbound traffic.
resource "omada_port_forwarding" "wireguard" {
  site_id       = "<site-id>"
  name          = "WireGuard"
  status        = true
  external_port = "51820"
  forward_ip    = "192.168.1.5"
  forward_port  = "51820"
  protocol      = 2 # UDP

  wan_port_ids = ["<primary-wan-port-id>"]

  # Empty means any external source. Populate this only when peers originate
  # from stable public addresses.
  source_addresses = []
}
