data "healthchecksio_channels" "email" {
  kind = "email"
}

# A simple check: one ping each hour, and a grace period of 10 minutes.
resource "healthchecksio_check" "backup" {
  name     = "Nightly backup"
  slug     = "nightly-backup"
  tags     = ["backup", "prod"]
  timeout  = 3600
  grace    = 600
  channels = data.healthchecksio_channels.email.ids
}

# A cron check: one ping at minute 42 of each hour (UTC), and a grace period of 3 hours.
resource "healthchecksio_check" "sync" {
  name     = "Mirror sync"
  slug     = "mirror-sync"
  schedule = "42 * * * *"
  tz       = "UTC"
  grace    = 10800
  channels = data.healthchecksio_channels.email.ids
}

# An OnCalendar check that accepts only POST pings, and fails a ping that does not
# contain "Backup successful".
resource "healthchecksio_check" "report" {
  name                = "Monthly report"
  schedule            = "*-*~1 12:00"
  methods             = "POST"
  success_kw          = ["Backup successful"]
  filter_http_body    = true
  filter_default_fail = true
}
