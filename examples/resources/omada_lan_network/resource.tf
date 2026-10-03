# Site ID to manage the network in. Provide via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

resource "omada_lan_network" "example" {
  site_id        = var.site_id
  name           = "IoT"
  vlan_id        = 100
  gateway_subnet = "192.168.100.1/24"
  domain         = "iot.local"

  dhcp_settings = {
    enable       = true
    dhcpns       = "manual"
    gateway      = "192.168.100.1"
    ipaddr_start = "192.168.100.100"
    ipaddr_end   = "192.168.100.250"
    leasetime    = 1440
    pri_dns      = "192.168.100.1"
    snd_dns      = "8.8.8.8"

    # Option 42 hands out NTP servers. type 1 = IP address.
    options = [
      { code = 42, type = 1, value = "192.168.100.1" },
    ]
  }

  # Keep IPv6 off: ipv6_enabled is read-only.
  lifecycle {
    postcondition {
      condition     = !self.ipv6_enabled
      error_message = "IPv6 was turned on for this network in the controller."
    }
  }
}

output "example_network_id" {
  value = omada_lan_network.example.network_id
}
