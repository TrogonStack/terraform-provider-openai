# Provider Configuration

## Overview

The provider is the top-level component that:

1. Defines its own configuration schema
2. Creates the shared client during Configure
3. Passes client data to resources and data sources
4. Registers all available resources and data sources

## Provider Interface

```go
type Provider interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Configure(context.Context, ConfigureRequest, *ConfigureResponse)
    Resources(context.Context) []func() resource.Resource
    DataSources(context.Context) []func() datasource.DataSource
}
```

## This Provider's Structure

### Metadata

```go
func (p *exampleProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
    resp.TypeName = "example"
    resp.Version = p.version
}
```

`TypeName` becomes the prefix for all resource names (e.g. a hypothetical `example_widget`, since no resource exists yet).

### Schema

Provider schema defines what goes in the `provider "example" {}` block. It has no attributes yet; a new provider adds its credentials and settings here:

```go
func (p *exampleProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
    resp.Schema = schema.Schema{
        MarkdownDescription: "Manage Example with Terraform.",
    }
}
```

### Configure

Configure creates the shared client and makes it available to resources. This is also real, from `provider.go`:

```go
func (p *exampleProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    client := newRetryableClient(nil)
    resp.DataSourceData = client
    resp.ResourceData = client
}
```

Unlike a provider that resolves credentials and can fail to authenticate, this provider needs none today: there is no configuration to read, and the shared client is built unconditionally, so the only way `Configure` fails at all is if a future change to it introduces one.

### Resource/DataSource Registration

```go
func (p *exampleProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{}
}

func (p *exampleProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{}
}
```

Both are empty today, since this provider has not shipped a resource or data source yet. Adding one means appending its constructor function here (see `references/guides/resource-lifecycle.md` and `references/guides/data-source-lifecycle.md`).

## Client Data Flow

```
provider.Configure()
    -> resp.ResourceData = client
    -> resp.DataSourceData = client

resource.Configure()
    -> req.ProviderData == client (same pointer)
    -> r.client = req.ProviderData.(*http.Client)

resource.Create/Read/Update/Delete()
    -> r.client.Do(httpReq)
```

The last call is illustrative (no resource exists yet); the shared `*http.Client` itself is real, built in `retry.go`:

```go
func newRetryableClient(base http.RoundTripper) *http.Client {
    if base == nil {
        base = http.DefaultTransport
    }
    // wraps base in the retry transport described below
}
```

There is no generated SDK and no per-resource service type: `*http.Client` is the entire surface a resource gets from the provider. A future resource builds its own `*http.Request` values and decodes JSON responses itself, rather than calling a typed method on a service object.

## Authentication

This provider has no authentication today. It has no provider-level attributes, no environment variable fallback, and no token source.

## Retry

`retry.go`'s `newRetryableClient` builds a `go-retryablehttp` client on top of the given base `http.RoundTripper` (defaulting to `http.DefaultTransport` when `nil`), with a custom `retryPolicy`: retry on connection errors, HTTP 429, and 5xx except 501 (Not Implemented, which retrying cannot fix); POST is never retried, and a cancelled context stops immediately. `Retry-After` is honored through `go-retryablehttp`'s own default backoff. A `methodTaggingTransport` tags the request's HTTP method onto the context so `retryPolicy` can tell a POST from anything else, and each attempt is bounded by a fixed `requestAttemptTimeout` of 90 seconds. The provider wires this in unconditionally; there is no provider-level attribute to configure it.

## Test Bypass

`provider_test.go` exercises `Configure` directly, without a bypass variable: since Configure makes no network calls and needs no credentials, calling it with an empty hand-built `tftypes.Value` is enough to get a real `*http.Client` back and assert that `resp.ResourceData` and `resp.DataSourceData` share it.

## Provider Server (main.go)

The entry point wraps the provider into a gRPC server:

```go
package main

import (
    "context"
    "log"

    "github.com/TrogonStack/terraform-provider-example/internal/provider"
    "github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version string = "dev"

func main() {
    err := providerserver.Serve(
        context.Background(),
        provider.New(version),
        providerserver.ServeOpts{
            Address: "registry.terraform.io/trogonstack/example",
        },
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

## Related Framework References

| File                                                                                                                            | Contents                                    |
| ------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| [providers/index](https://developer.hashicorp.com/terraform/plugin/framework/providers)                                         | Provider interface, metadata, schema        |
| [providers/validate-configuration](https://developer.hashicorp.com/terraform/plugin/framework/providers/validate-configuration) | Provider-level validation                   |
| [provider-servers](https://developer.hashicorp.com/terraform/plugin/framework/provider-servers)                                 | Server setup, protocol versions, debug mode |
| [resources/configure](https://developer.hashicorp.com/terraform/plugin/framework/resources/configure)                           | How resources receive provider data         |
| [data-sources/configure](https://developer.hashicorp.com/terraform/plugin/framework/data-sources/configure)                     | How data sources receive provider data      |
