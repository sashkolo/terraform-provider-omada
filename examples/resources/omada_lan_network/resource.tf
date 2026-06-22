# Site ID to manage the network in. Provide via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

resource "omada_lan_network" "example" {
  site_id        = var.site_id
  name           = "IoT"
  vlan_id        = 30
  gateway_subnet = "192.168.30.1/24"
  domain         = "iot.local"

  dhcp_settings = {
    enable       = true
    dhcpns       = "manual"
    gateway      = "192.168.30.1"
    ipaddr_start = "192.168.30.100"
    ipaddr_end   = "192.168.30.250"
    leasetime    = 1440
    pri_dns      = "192.168.30.1"
    snd_dns      = "8.8.8.8"
  }
}

output "example_network_id" {
  value = omada_lan_network.example.network_id
}
