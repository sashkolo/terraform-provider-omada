# Site ID and WLAN group that own the SSID. Provide these via variables or data
# sources so the values are not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

variable "wlan_group_id" {
  type = string
}

variable "ssid_psk" {
  type      = string
  sensitive = true
}

resource "omada_ssid" "iot" {
  site_id       = var.site_id
  wlan_group_id = var.wlan_group_id
  name          = "IoT"
  security      = 3 # WPA-Personal
  band          = 3 # 2.4G + 5G
  broadcast     = true

  # Tag IoT client traffic onto the IoT VLAN (managed by omada_lan_network).
  vlan_enable = true
  vlan_id     = 30

  psk_setting = {
    psk = var.ssid_psk
  }
}

output "iot_ssid_id" {
  value = omada_ssid.iot.ssid_id
}
