data "healthchecksio_badges" "all" {}

output "prod_badge" {
  value = data.healthchecksio_badges.all.badges["prod"].svg3
}
