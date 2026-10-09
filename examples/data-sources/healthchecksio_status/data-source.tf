data "healthchecksio_status" "server" {}

resource "healthchecksio_check" "backup" {
  name = "Nightly backup"

  lifecycle {
    precondition {
      condition     = data.healthchecksio_status.server.ok
      error_message = "The healthchecks server cannot query its database."
    }
  }
}
