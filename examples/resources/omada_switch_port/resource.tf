# Site ID and switch MAC the port belongs to. Provide the site via a variable or
# data source so the value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# Pin a single switch port to an access/untagged profile. For a non-stacked
# switch, port 8 is the UI's "1/0/8". The port's VLAN posture is governed by the
# referenced profile, so an access/untagged profile with no tagged VLANs keeps an
# exposed port off every sensitive VLAN.
resource "omada_switch_port" "outdoor" {
  site_id    = var.site_id
  switch_mac = "E4-FA-C4-9E-CD-87"
  port       = 8
  profile_id = "<outdoor-untrusted-profile-id>"
}
