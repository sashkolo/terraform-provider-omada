# Site ID that owns the settings. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# Attack-defense settings are a singleton per site: import the live object first
# (`tofu import omada_attack_defense_setting.default <site_id>`) so the baseline
# plan is a no-op before managing it. The whole object is overwritten on apply.
resource "omada_attack_defense_setting" "default" {
  site_id = var.site_id

  # DoS / flood / scan protection enables + per-category rate limits.
  ping_death_enable      = true
  ping_wan_enable        = false # do not answer ping from the WAN
  win_nuke_attack_enable = true
  large_ping_enable      = true
  large_ping_threshold   = 1024

  icmp_conn_enable = true
  icmp_conn_limit  = 300
  icmp_src_enable  = true
  icmp_src_limit   = 300

  tcp_conn_enable       = true
  tcp_conn_limit        = 300
  tcp_src_enable        = true
  tcp_src_limit         = 300
  tcp_scan_enable       = true
  tcp_scan_reject       = true
  tcp_fin_no_ack_enable = true
  tcp_syn_fin_enable    = true

  udp_conn_enable = true
  udp_conn_limit  = 300
  udp_src_enable  = true
  udp_src_limit   = 300

  specified_option_enable = false
  # specified_option = { ... } # per-IP-option toggles, when enabled
}
