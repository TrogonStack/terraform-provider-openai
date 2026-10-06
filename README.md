# terraform-provider-openai

A Terraform provider for the [OpenAI Admin API](https://platform.openai.com/docs/api-reference/administration). It manages organization projects and the service accounts inside them. It is not affiliated with or endorsed by OpenAI.

## Provider configuration

```hcl
terraform {
  required_providers {
    openai = {
      source = "TrogonStack/openai"
    }
  }
}

provider "openai" {}
```

| Attribute | Environment variable | Default | Description |
|---|---|---|---|
| `admin_api_key` | `OPENAI_ADMIN_KEY` | none | An Admin API key for the organization. Sensitive. The provider fails to configure without one. |
| `base_url` | `OPENAI_BASE_URL` | `https://api.openai.com/v1` | The base URL of the Admin API. |

A value set in the provider block takes precedence over the environment variable. Admin API keys are created by an organization owner in the OpenAI platform settings. A regular project API key does not work.

## Resources

| Resource | Description |
|---|---|
| [`openai_project`](docs/resources/project.md) | An organization project. |
| [`openai_project_service_account`](docs/resources/project_service_account.md) | A service account in a project, with the API key created for it. |

### Destroying a project archives it

The Admin API cannot delete a project. Destroying an `openai_project` archives it instead, and an archived project cannot be restored through the API. A project archived outside Terraform is removed from state and created again on the next apply.

### A service account's API key is available only at creation

The Admin API returns the secret value of a service account's API key once, in the response that creates the service account. The provider stores it in the sensitive `api_key` attribute and keeps it across refreshes. A service account brought in with `terraform import` has a null `api_key`.

To rotate the key, replace the service account:

```bash
terraform apply -replace="openai_project_service_account.example"
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup, test workflow, and release process.
