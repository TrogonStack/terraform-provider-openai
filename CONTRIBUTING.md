# Contributing

## Prerequisites

- [mise](https://mise.jdx.dev), which pins the Go, `golangci-lint`, GoReleaser, and `tfplugindocs` versions used by CI

Install the toolchain with `mise install`. Every command below runs through `mise` so local runs match CI.

Coding agents get the provider-specific skill from `.agents/skills/terraform-provider-dev`. For deeper Terraform Plugin Framework guidance, HashiCorp's skills can be installed at the user level, outside this repository, since they are MPL-2.0 licensed:

```bash
npx skills add hashicorp/agent-skills --global
```

## Development

```bash
mise run build      # full CI pipeline: download, lint, test, tidy, docs, diff
mise run test       # go test -count=1 -cover -skip TestLive ./...
mise run test:live  # live tests against a real API
mise run lint       # golangci-lint run --fix ./...
mise run docs       # regenerate docs/ from schema descriptions
```

`mise run build` is what CI runs on every pull request, including the `git diff --exit-code` check, so run it before pushing.

## Testing

Unit tests never reach a real API. `internal/provider/retry_test.go` serves canned responses from an `httptest.Server` and drives the retrying client in `retry.go` directly against it.

```bash
mise exec -- go test ./internal/provider/ -v -run TestRetry
```

Live tests are named `TestLive_*` and skip unless `TF_ACC` and `EXAMPLE_TEST_ENDPOINT` are set. Run them with `mise run test:live` once the provider has a resource worth exercising against a real API.

## Code layout

All code lives in the flat `internal/provider/` package. Resources go in `resource_<name>.go` and data sources in `data_source_<name>.go`, with tests alongside as `<file>_test.go`. New resources must be registered in the `Resources()` or `DataSources()` method in `provider.go`, or the provider will not expose them.

The provider's HTTP client, built in `retry.go`, wraps every request in a retry policy: automatic retry on connection errors, 429, and 5xx except 501, honoring `Retry-After`, with a 90 second timeout per attempt. POST is never retried, since a create can fail after it has already taken effect.

## Commits

Commits follow [Conventional Commits](https://www.conventionalcommits.org) and require a [DCO](https://developercertificate.org) sign-off:

```bash
git commit -s -m "fix(client): retry 503 responses on idempotent methods only"
```

The commit type determines the next version, so it is worth getting right.

## Releases

[release-please](https://github.com/googleapis/release-please) reads the conventional commits merged into `main` and maintains an open release pull request with the computed version bump and changelog entries. Merging that pull request tags the release and publishes the provider archives, plus a GPG-signed checksum file, via [GoReleaser](https://goreleaser.com). No release happens without that pull request being merged.

Each release carries the assets the provider registry protocol expects: one zip per platform, a `SHA256SUMS` file, a detached GPG signature over it, and `terraform-provider-example_<version>_manifest.json` built from `terraform-registry-manifest.json` at the repository root. That manifest declares plugin protocol 6, which `providerserver.Serve` uses because `main.go` leaves `ProtocolVersion` unset. Registries assume protocol 5.0 when the manifest is missing, so a release without it installs and then fails to load.
