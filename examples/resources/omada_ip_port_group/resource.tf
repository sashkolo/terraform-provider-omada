# Site ID that owns the group. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# A reusable IP-Port group: IP hosts/subnets scoped to a set of service ports.
# ACLs reference it by group id (source_type/destination_type = 2). Here: the
# camera subnet restricted to the ONVIF/RTSP/HTTP(S) service ports, so a
# single allow rule can express "reach the cameras only on these ports".
resource "omada_ip_port_group" "camera_service_ports" {
  site_id     = var.site_id
  name        = "camera-service-ports"
  description = "Camera ONVIF/RTSP/HTTP(S) service ports"

  ip_list = [
    { ip = "192.168.100.0", mask = 24, description = "camera subnet" },
  ]

  # port_type defaults to 0 (port-list mode). Each entry is a single port or an
  # inclusive range ("8000-8100").
  port_list = ["80", "443", "554", "2020"]
}

# Reference the group id from a gateway ACL:
#
#   resource "omada_acl" "example" {
#     # ...
#     destination_type = 2 # IP-Port Group
#     destination_ids  = [omada_ip_port_group.camera_service_ports.group_id]
#   }

output "camera_service_ports_group_id" {
  value = omada_ip_port_group.camera_service_ports.group_id
}
