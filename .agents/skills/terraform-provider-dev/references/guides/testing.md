# Testing

## Overview

This provider's tests never reach a real backend, except the `TestLive_*` tests gated behind `TF_ACC`. There are two layers today:

1. **Provider configuration tests** (`provider_test.go`): exercise `exampleProvider.Configure` directly, without building a Terraform config string or running `resource.Test`
2. **Retry policy tests** (`retry_test.go`): unit tests of the `retryPolicy` function plus `newRetryableClient`'s end-to-end retry behavior over a real HTTP server

No resource or data source exists yet, so there is no `resource.Test`-based acceptance test harness, no `client_test.go`, and no `helpers_test.go`: there is no generated SDK client, and no credentials to fake. The final section below describes the pattern a future resource's tests would follow.

## Provider Configuration Tests

`provider_test.go` builds a provider, drives `Configure` with a hand-built `tftypes.Value`, and asserts on the response, entirely without Terraform's test driver:

```go
func configureProvider(t *testing.T) *provider.ConfigureResponse {
    t.Helper()
    ctx := context.Background()
    p := New("test")()

    schemaResp := &provider.SchemaResponse{}
    p.Schema(ctx, provider.SchemaRequest{}, schemaResp)

    raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{})

    resp := &provider.ConfigureResponse{}
    p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
    return resp
}
```

This mirrors the real `provider_test.go`, asserting that `resp.ResourceData` is an `*http.Client` and that `resp.DataSourceData` holds that exact same pointer.

## Retry Policy Tests

`retry_test.go` has two kinds of tests. The policy function itself, `retryPolicy`, is tested directly with no server involved, since it's a plain `func(context.Context, *http.Response, error) (bool, error)`:

```go
func TestRetryPolicy_429_Retries(t *testing.T) {
    resp := &http.Response{StatusCode: 429}
    retry, err := retryPolicy(context.Background(), resp, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !retry {
        t.Error("expected retry on 429")
    }
}
```

A table covering the policy's documented behavior: a 429 retries, a 5xx retries, a 501 does not, a 2xx does not, a connection error (a non-nil `err` with no response) retries, a cancelled context does not retry, and a POST never retries regardless of status code, with the request method tagged onto the context the same way `methodTaggingTransport` does in production.

The second kind drives `newRetryableClient` end-to-end against a real `httptest.Server`, since the actual retrying happens in the composed `*http.Client`, not in `retryPolicy` alone: bounding each attempt to the fixed per-attempt timeout, never retrying POST, retrying idempotent methods like GET and DELETE, and exhausting after the configured number of retries.

## Live Acceptance Tests

`live_test.go` is a placeholder today, `TestLive_Placeholder`, skipped unless both `TF_ACC` and `EXAMPLE_TEST_ENDPOINT` are set:

```go
func requireLiveEndpoint(t *testing.T) string {
    t.Helper()
    if os.Getenv("TF_ACC") == "" {
        t.Skip("set TF_ACC=1 to run live acceptance tests")
    }
    endpoint := os.Getenv("EXAMPLE_TEST_ENDPOINT")
    if endpoint == "" {
        t.Skip("set EXAMPLE_TEST_ENDPOINT to run live acceptance tests")
    }
    return endpoint
}

func TestLive_Placeholder(t *testing.T) {
    requireLiveEndpoint(t)
    t.Skip("replace with a real live test once a resource exists")
}
```

This is a starting point to replace once the first real resource exists, not a test of any real behavior today. `mise run test` runs the full suite skipping `TestLive_*`, so this never runs by accident; `mise run test:live` runs it explicitly.

## Testing a Future Resource

No resource exists yet, so this is illustrative. For a hypothetical `example_widget`:

1. Stand up an `httptest.Server` with handlers for the paths it calls (create, get, update, delete, keyed by ID), backed by an in-memory map
2. Point the provider at it through a provider attribute (such as a base URL) in the test configuration, so the resource's requests through the shared `*http.Client` land on the fake server. Adding that attribute and wiring it through to a resource's own requests is part of the work of adding the first resource, since today `ProviderData` is only the client itself
3. Add a provider factory map and minimal provider config block, mirroring other Terraform Plugin Framework providers:

```go
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
    "example": providerserver.NewProtocol6WithError(New("test")()),
}

const testProviderConfig = `
provider "example" {}
`
```

4. Drive `resource.Test` the usual way:

```go
func TestAccWidget_Basic(t *testing.T) {
    // set up the httptest.Server and point the provider at it as described above

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testProviderConfig + `
resource "example_widget" "test" {
  name = "premium"
}
`,
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttr("example_widget.test", "name", "premium"),
                    resource.TestCheckResourceAttrSet("example_widget.test", "id"),
                ),
            },
        },
    })
}
```

5. For a not-found/drift test, delete the item from the in-memory map between steps and use `PlanOnly` + `ExpectNonEmptyPlan`
6. For import, add an `ImportState: true, ImportStateVerify: true` step; if a field cannot be reconstructed from a read (write-only, or not returned by the API), add it to `ImportStateVerifyIgnore`

## Check Functions

| Function                                                       | Use Case                           |
| -------------------------------------------------------------- | ---------------------------------- |
| `resource.TestCheckResourceAttr(name, key, value)`             | Exact attribute match              |
| `resource.TestCheckResourceAttrSet(name, key)`                 | Attribute is set (any value)       |
| `resource.TestCheckNoResourceAttr(name, key)`                  | Attribute is NOT set               |
| `resource.TestCheckTypeSetElemAttr(name, key, value)`          | Element present in a Set attribute |
| `resource.TestCheckResourceAttrPair(name1, key1, name2, key2)` | Two attributes match               |
| `resource.ComposeAggregateTestCheckFunc(...)`                  | Combine multiple checks            |

## Running Tests

```bash
go test ./internal/provider/ -v -run TestProviderConfigure
go test ./internal/provider/ -v -run TestRetryPolicy

# Full suite, matching `mise run test` (skips TestLive_*)
go test -count=1 -cover -skip TestLive ./...

# Live acceptance tests
mise run test:live
```

## Test Naming Convention

```
Test<Subject>_<Scenario>
```

Examples from this provider: `TestRetryPolicy_429_Retries`, `TestRetryableClient_RetriesIdempotentMethods`, `TestLive_Placeholder`. A future resource's acceptance tests would follow `TestAcc<Resource>_<Scenario>` (e.g. `TestAccWidget_Basic`), matching the Plugin Framework's own convention.

## Related Framework References

| File                                                                              | Contents                             |
| --------------------------------------------------------------------------------- | ------------------------------------ |
| [acctests](https://developer.hashicorp.com/terraform/plugin/framework/acctests)   | Acceptance test setup with framework |
| [debugging](https://developer.hashicorp.com/terraform/plugin/framework/debugging) | Debugging test failures              |
