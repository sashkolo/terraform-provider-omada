# Site ID that owns the group. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# A reusable IP group: one or more hosts/subnets an ACL can reference by group id
# (source_type/destination_type = 1) instead of hard-coding whole-VLAN networks.
# Here: two camera hosts on the untrusted VLAN.
resource "omada_ip_group" "camera_hosts" {
  site_id     = var.site_id
  name        = "camera-hosts"
  description = "Camera hosts"

  ip_list = [
    { ip = "192.168.100.51", mask = 32, description = "cam-porch" },
    { ip = "192.168.100.52", mask = 32, description = "cam-drive" },
  ]
}

# Reference the group id from a gateway ACL:
#
#   resource "omada_acl" "example" {
#     # ...
#     destination_type = 1 # IP Group
#     destination_ids  = [omada_ip_group.camera_hosts.group_id]
#   }

output "camera_hosts_group_id" {
  value = omada_ip_group.camera_hosts.group_id
}
