data "healthchecksio_channels" "ops" {
  name = "Ops email"
}

resource "healthchecksio_check" "backup" {
  name     = "Nightly backup"
  channels = data.healthchecksio_channels.ops.ids
}
