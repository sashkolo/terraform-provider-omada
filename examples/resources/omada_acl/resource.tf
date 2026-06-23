# Site ID that owns the ACL. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# A permissive, reversible probe rule: allow TCP from the IoT LAN network to the
# IoT LAN network (LAN-to-LAN). Use a dedicated, non-colliding source/destination
# and disable the rule (status = false) until the change is reviewed. ACLs are
# the core of inter-VLAN segmentation and can sever connectivity if misapplied.
resource "omada_acl" "probe" {
  site_id     = var.site_id
  description = "TF Probe ACL"
  status      = false # disabled until reviewed
  syslog      = false

  source_type      = 0 # network
  source_ids       = ["<iot-lan-network-id>"]
  destination_type = 0 # network
  destination_ids  = ["<iot-lan-network-id>"]
  policy           = 1   # allow
  protocols        = [6] # TCP
  state_mode       = 0   # auto

  direction = {
    lan_to_lan = true
  }
}

output "probe_acl_id" {
  value = omada_acl.probe.acl_id
}
