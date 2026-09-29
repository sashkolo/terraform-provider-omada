# Site ID that owns the profile. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# An access/untagged switch port profile on a single VLAN: a native network and
# zero tagged networks. This is the shape used to keep an exposed port pinned to
# one untrusted VLAN with no trunked/sensitive VLANs.
resource "omada_switch_port_profile" "untrusted" {
  site_id            = var.site_id
  name               = "Untrusted"
  native_network_id  = "<untrusted-lan-network-id>" # VLAN this profile pins ports to
  tagged_network_ids = []                           # access port: no trunked VLANs
}

output "untrusted_profile_id" {
  value = omada_switch_port_profile.untrusted.profile_id
}
