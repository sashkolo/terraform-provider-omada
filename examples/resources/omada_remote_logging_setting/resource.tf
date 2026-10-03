# Site ID that owns the setting. Provide it via a variable or data source so the
# value is not hard-coded in checked-in configurations.
variable "site_id" {
  type = string
}

# Remote logging is a singleton per site: import the live setting first
# (`tofu import omada_remote_logging_setting.default <site_id>`) so the baseline
# plan is a no-op before managing it. Destroy leaves the live setting alone.
resource "omada_remote_logging_setting" "default" {
  site_id = var.site_id
  enable  = true
  host    = "192.0.2.10" # the syslog collector
  port    = 514

  # Optional: whether client logs are included. Unset keeps the live value.
  # more_client_log = false
}
