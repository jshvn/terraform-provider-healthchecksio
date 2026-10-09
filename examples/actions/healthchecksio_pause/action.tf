action "healthchecksio_pause" "maintenance" {
  config {
    check_id = healthchecksio_check.backup.id
  }
}

# To start the action from the CLI: terraform apply -invoke=action.healthchecksio_pause.maintenance
