# Site ID that owns the settings. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# The firewall settings are a singleton per site: import the live object first
# (`tofu import omada_firewall_setting.default <site_id>`) so the baseline plan
# is a no-op before managing it. The whole object is overwritten on apply.
resource "omada_firewall_setting" "default" {
  site_id = var.site_id

  # Hardening toggles (security-relevant; set deliberately).
  broadcast_ping    = false # drop broadcast ping
  receive_redirects = false # ignore ICMP redirects
  send_redirects    = false # do not send ICMP redirects
  syn_cookies       = true  # SYN-flood cookie protection

  # Conntrack/protocol timeouts (seconds, 1-2097151).
  icmp            = 30
  other           = 600
  tcp_close       = 10
  tcp_close_wait  = 60
  tcp_established = 7440
  tcp_fin_wait    = 120
  tcp_last_ack    = 30
  tcp_syn_receive = 60
  tcp_syn_sent    = 120
  tcp_time_wait   = 120
  udp_other       = 60
  udp_stream      = 180
}
