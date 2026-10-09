terraform {
  required_providers {
    healthchecksio = {
      source  = "jshvn/healthchecksio"
      version = "~> 1.0"
    }
  }
}

# The provider reads the API key from HEALTHCHECKSIO_API_KEY. Do not write a key in a file.
provider "healthchecksio" {}

# Each project has its own API keys. For a second project, use a second provider block
# with an alias.
provider "healthchecksio" {
  alias   = "backups"
  api_key = var.backups_api_key
}

variable "backups_api_key" {
  type      = string
  sensitive = true
}
