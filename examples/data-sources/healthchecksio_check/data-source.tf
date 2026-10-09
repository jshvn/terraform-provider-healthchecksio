data "healthchecksio_check" "backup" {
  slug = "nightly-backup"
}

output "backup_status" {
  value = data.healthchecksio_check.backup.status
}
