data "healthchecksio_ping_body" "last" {
  check_id = healthchecksio_check.backup.id
  n        = data.healthchecksio_pings.backup.pings[0].n
}
