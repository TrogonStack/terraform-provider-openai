# terraform-provider-example

**A template Terraform provider.** It ships an empty provider configuration, a retrying HTTP client, and no resources or data sources yet. It builds, lints, and tests green out of the box, and is meant to be copied, not used directly.

**Resources are added one at a time, as they are needed.** This template exists so a new provider repository starts from working tooling (mise tasks, CI, release automation, linting) instead of from a blank directory.

## Provider configuration

```hcl
provider "example" {}
```

## Resources

None yet.

## Using this template

To start a new provider from this template:

1. Create the new repository from this template.
2. Rename `example` to the new provider's name everywhere it appears: the Go module path in `go.mod` and every import of it, the registry address in `main.go`, the provider type name and schema description in `internal/provider/provider.go`, the `provider-name` passed to `tfplugindocs` in `mise.toml`, `package-name` in `.github/release-please-config.json`, the example in `examples/provider/provider.tf`, and every mention of `terraform-provider-example` in this file, `CONTRIBUTING.md`, and `AGENTS.md`.
3. Update `.github/terraform-registry.json`'s `category` to whatever best describes the new provider in the Terraform Registry.
4. Replace this README's description, the provider configuration example, and the "Resources" section as resources are added.
5. Run `mise trust && mise run build` and confirm it is still green before the first commit.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup, test workflow, and release process.
