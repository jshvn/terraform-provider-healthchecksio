data "healthchecksio_checks" "prod" {
  tags = ["prod"]
}

output "down" {
  value = [for c in data.healthchecksio_checks.prod.checks : c.name if c.status == "down"]
}
