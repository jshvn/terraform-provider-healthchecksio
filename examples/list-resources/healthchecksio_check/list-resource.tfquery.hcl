# Find the checks with the tag "prod". Then, to write an import block and a resource
# block for each check, run: terraform query -generate-config-out=checks.tf
list "healthchecksio_check" "prod" {
  provider         = healthchecksio
  include_resource = true

  config {
    tags = ["prod"]
  }
}
