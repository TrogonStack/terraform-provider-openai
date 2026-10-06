resource "openai_project" "example" {
  name = "example-project"
}

resource "openai_project_service_account" "example" {
  project_id = openai_project.example.id
  name       = "example-service-account"
}

output "example_service_account_api_key" {
  value     = openai_project_service_account.example.api_key
  sensitive = true
}
