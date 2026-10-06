# terraform-provider-openai

Template Terraform provider. Resources and data sources are added one at a time as they are needed; copy this repository to start a new provider, then rename `example` throughout (see "Using this template" in README.md).

- **Module**: `github.com/TrogonStack/terraform-provider-openai`
- **Package**: `internal/provider/` (single flat package, all resources here)

## Commands

```bash
mise run test          # go test -count=1 -cover -skip TestLive ./...
mise run test:live     # TestLive_* against a real API
mise run lint          # golangci-lint run --fix ./...
mise run build         # full CI pipeline (download, tidy, lint, test, docs, diff)
mise run docs          # regenerate docs/ from schema descriptions
```

Single test:

```bash
go test ./internal/provider/ -v -run TestRetry
```

## Skills

Always load `terraform-provider-dev` when working on a resource or data source.

## Architecture

- File naming: `resource_<name>.go`, `resource_<name>_test.go`, `data_source_<name>.go`
- Provider client: `*http.Client`, built in `retry.go` and injected into every resource and data source through `Configure`'s `req.ProviderData`
- Auth: none yet. A new provider typically resolves credentials in a `client.go` and layers them onto the transport `newRetryableClient` wraps, the same way the base transport in `retry.go` is pluggable today
- New resources must be registered in `provider.go` `Resources()` / `DataSources()`

## Conventions

### Retry

Automatic retry on connection errors, 429, and 5xx except 501, honoring `Retry-After`, with a 90 second timeout per attempt. POST is never retried, since a create can fail after it has already taken effect. No configuration attribute: the transport in `retry.go` is fixed.

### Context propagation

Every outgoing request should carry the `ctx` passed into Terraform's CRUD methods, so cancellation reaches in-flight requests.

## CI

- PR and push to main: lint + test + build (GitHub Actions)
- Release: release-please + goreleaser on push to main, skipped in the template repository itself
