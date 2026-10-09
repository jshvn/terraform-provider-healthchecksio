# The status changes of the last 7 days.
data "healthchecksio_flips" "backup" {
  check_id = healthchecksio_check.backup.id
  seconds  = 604800
}
