data "healthchecksio_pings" "backup" {
  check_id = healthchecksio_check.backup.id
}

output "last_ping_type" {
  value = data.healthchecksio_pings.backup.pings[0].type
}
