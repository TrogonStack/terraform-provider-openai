terraform {
  required_providers {
    openai = {
      source = "TrogonStack/openai"
    }
  }
}

# Leave admin_api_key unset to read it from the OPENAI_ADMIN_KEY environment variable.
provider "openai" {
  admin_api_key = var.openai_admin_api_key
}

variable "openai_admin_api_key" {
  type      = string
  sensitive = true
}
