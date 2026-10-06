---
name: terraform-provider-dev
description: >
  Use this skill when developing terraform-provider-example: adding
  resources or data sources, designing schemas, implementing CRUD operations,
  plan modification, state upgrades, import, validation, acceptance testing,
  debugging, or any Terraform Plugin Framework work in Go. Also use when the
  user asks about terraform provider patterns, attribute types, or how to
  structure tests. This is the primary development skill for this repository.
---

# Terraform Provider Development (Plugin Framework)

## Mental Model

- Provider = Go server implementing Terraform RPCs (GetProviderSchema, PlanResourceChange, ApplyResourceChange, ReadResource, etc.)
- Resource = struct implementing `resource.Resource` interface: Metadata, Schema, Configure, Create, Read, Update, Delete
- DataSource = struct implementing `datasource.DataSource` interface: Metadata, Schema, Configure, Read
- Schema defines the "shape" of config/plan/state: attributes (leaf values) and blocks (nested structures)
- Plan then Apply: Terraform calls PlanResourceChange (propose changes), then ApplyResourceChange (execute)
- State = Terraform's record of the real world; Plan = expected post-apply state
- Computed attributes: set by the provider from API responses (IDs, timestamps, server-generated values)
- Plugin Framework uses strong Go types: `types.String`, `types.Bool`, `types.Int64`, `types.List`, etc.
- Null vs Unknown: null means the user did not set it; unknown means the value will be known after apply (planned computed)

---

## This Provider: Conventions

This provider ships the provider configuration and a shared HTTP client today; it has no resources or data sources yet (`Resources()` and `DataSources()` in `provider.go` both return an empty slice). Everything under "Adding a New Resource" and "Adding a Data Source" below is illustrative, built around a hypothetical `example_widget` resource with a single `name` attribute.

- **Package**: `internal/provider` (single flat package, all resources will live here)
- **File naming**: `resource_<name>.go`, `resource_<name>_test.go`, `data_source_<name>.go`
- **Provider client**: a plain `*http.Client`, built by `newRetryableClient` in `retry.go`. There is no generated SDK and no hand-written service wrapper: a future resource makes its own HTTP requests with this client and decodes the response itself
- **Client injection**: Configure method casts `req.ProviderData.(*http.Client)`
- **Auth**: none today. The provider has no attributes, no environment-variable fallback, and no token source
- **ID helper**: no `helpers.go` exists yet, since no resource has been added. When the first resource lands, give it a Computed `id` attribute with `stringplanmodifier.UseStateForUnknown()` if the API has one distinct from its natural key, following the pattern described in `references/guides/schema-design.md`
- **Registration**: new resources and data sources are added to `Resources()` / `DataSources()` in `provider.go`, both currently empty
- **Not-found handling**: no `errors.go` and no shared `isNotFound` helper exist yet, since there is no API to call. A future resource checks the HTTP status code of its own response directly; once a second resource needs the same check, that is the point to extract a shared helper
  - **Read**: call `resp.State.RemoveResource(ctx)` and return (resource was deleted externally)
  - **Delete**: return without error where the API has no delete and documents a no-op instead
- **Retry**: `retry.go` wraps the client's HTTP transport with automatic retry on connection errors, 429, and 5xx except 501, honoring `Retry-After`, and never retries POST. No configuration attribute
- **Testing**: no resource tests exist yet. `provider_test.go` exercises `Configure` directly (schema plus a hand-built `tftypes.Value`), asserting `resp.ResourceData` is an `*http.Client` shared with `resp.DataSourceData`. `TestLive_Placeholder` in `live_test.go` is a starting point to replace once a real resource exists, skipped unless `TF_ACC` and `EXAMPLE_TEST_ENDPOINT` are set

---

## Adding a New Resource

1. Create `internal/provider/resource_<name>.go`
2. Define model struct(s) with `tfsdk` tags
3. Implement the resource:

```go
var (
    _ resource.Resource                = &widgetResource{}
    _ resource.ResourceWithImportState = &widgetResource{}
)

func newWidgetResource() resource.Resource { return &widgetResource{} }

type widgetResource struct {
    client *http.Client
}

type widgetResourceModel struct {
    ID   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}

func (r *widgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_widget"
}

func (r *widgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id": schema.StringAttribute{
                Computed:      true,
                PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
            },
            "name": schema.StringAttribute{
                Required:      true,
                PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
            },
        },
    }
}

func (r *widgetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*http.Client)
    if !ok {
        resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *http.Client, got: %T", req.ProviderData))
        return
    }
    r.client = client
}
```

4. Implement Create, Read, Update, Delete (see guide: `references/guides/resource-lifecycle.md`)
5. Implement ImportState
6. Register in `provider.go`: add `newWidgetResource` to `Resources()` return slice
7. Create `internal/provider/resource_widget_test.go` (see guide: `references/guides/testing.md`)

---

## Adding a Data Source

The provider does not define any data sources yet (`DataSources()` returns an empty slice). The shape below is illustrative, built around the same hypothetical `example_widget` resource, reading it back by ID:

```go
var _ datasource.DataSource = &widgetDataSource{}

func newWidgetDataSource() datasource.DataSource { return &widgetDataSource{} }

type widgetDataSource struct {
    client *http.Client
}

type widgetDataSourceModel struct {
    ID   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}

func (d *widgetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_widget"
}

func (d *widgetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":   schema.StringAttribute{Required: true},
            "name": schema.StringAttribute{Computed: true},
        },
    }
}

func (d *widgetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*http.Client)
    if !ok {
        resp.Diagnostics.AddError("Unexpected DataSource Configure Type", fmt.Sprintf("Expected *http.Client, got: %T", req.ProviderData))
        return
    }
    d.client = client
}

func (d *widgetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
    var data widgetDataSourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/widgets/"+data.ID.ValueString(), nil)
    if err != nil {
        resp.Diagnostics.AddError("Request Error", fmt.Sprintf("Unable to build request: %s", err))
        return
    }

    httpResp, err := d.client.Do(httpReq)
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read widget: %s", err))
        return
    }
    defer httpResp.Body.Close()

    var found struct {
        Name string `json:"name"`
    }
    if err := json.NewDecoder(httpResp.Body).Decode(&found); err != nil {
        resp.Diagnostics.AddError("Response Error", fmt.Sprintf("Unable to decode response: %s", err))
        return
    }

    data.Name = types.StringValue(found.Name)
    resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
```

Register: add `newWidgetDataSource` to `DataSources()` in `provider.go`.

---

## Schema Design Quick-Reference

| Schema Type                                              | Go Model Type  | When to Use                                            |
| -------------------------------------------------------- | -------------- | ------------------------------------------------------ |
| `schema.StringAttribute{Required: true}`                 | `types.String` | User must provide                                      |
| `schema.StringAttribute{Optional: true}`                 | `types.String` | User may provide                                       |
| `schema.StringAttribute{Computed: true}`                 | `types.String` | Server-generated only                                  |
| `schema.StringAttribute{Optional: true, Computed: true}` | `types.String` | User provides OR server fills                          |
| `schema.StringAttribute{Sensitive: true}`                | `types.String` | A value that should not appear in plan or state output |

The table above is illustrative until the first attribute exists. A hypothetical `example_widget` would mark `name` Required and `id` Computed, since the identifier is assigned by the server rather than something the caller sets directly.

### Plan Modifiers

| Modifier                                  | Use Case                                                                                     |
| ----------------------------------------- | -------------------------------------------------------------------------------------------- |
| `stringplanmodifier.UseStateForUnknown()` | Computed value stable across updates (an `id` attribute)                                     |
| `stringplanmodifier.RequiresReplace()`    | Changing this forces resource recreation (immutable fields, e.g. `name` on `example_widget`) |

Full details: `references/guides/schema-design.md`

---

## Testing Patterns

### Test Infrastructure

No acceptance-test harness (`resource.Test`, a provider factory map) exists yet, since there are no resources to exercise that way. What exists today is a direct unit test of the provider itself:

```go
func configureProvider(t *testing.T) *provider.ConfigureResponse {
    t.Helper()
    ctx := context.Background()
    p := New("test")()

    schemaResp := &provider.SchemaResponse{}
    p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
    // build an empty tftypes.Value for the schema and call p.Configure(...)
}
```

`provider_test.go` exercises this directly, driving `Configure` with a hand-built `tftypes.Value` and asserting that `resp.ResourceData` is an `*http.Client` shared with `resp.DataSourceData`, without ever building a Terraform config string.

### Test Structure (what a future resource test will look like)

A future resource test would stand up an `httptest.Server`, point the provider at it through a provider attribute added for that purpose, and drive the result through `resource.Test` once the first resource exists. There is no generated SDK to build a fake client against, so the resource's own HTTP calls hit the test server directly through the shared `*http.Client`.

### Running Tests

```bash
go test ./internal/provider/ -v -run TestProviderConfigure
mise run test:live   # TestLive_* against EXAMPLE_TEST_ENDPOINT
```

Full details: `references/guides/testing.md`

---

## State Upgrade

No resource exists yet, so none has needed a state upgrade. If a future resource's schema needs a breaking change after it ships (e.g. splitting a flat attribute into a nested object):

1. Increment `Version` in the schema
2. Implement `resource.ResourceWithUpgradeState`
3. Parse raw JSON state and write to current model

```go
func (r *widgetResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
                var raw map[string]json.RawMessage
                if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
                    resp.Diagnostics.AddError("State Upgrade Error", fmt.Sprintf("Unable to parse raw state: %s", err))
                    return
                }
                // Parse old format, build new model, set state
                resp.Diagnostics.Append(resp.State.Set(ctx, &newModel)...)
            },
        },
    }
}
```

Full details: `references/guides/state-management.md`

---

## Reference Docs

### Topic Guides (synthesized, task-oriented)

| Guide                                         | Contents                                              |
| --------------------------------------------- | ----------------------------------------------------- |
| `references/guides/resource-lifecycle.md`     | CRUD methods, interface contracts, registration       |
| `references/guides/data-source-lifecycle.md`  | Data source pattern, Read method                      |
| `references/guides/schema-design.md`          | Attributes, blocks, types, nested models              |
| `references/guides/plan-modification.md`      | UseStateForUnknown, RequiresReplace, custom modifiers |
| `references/guides/state-management.md`       | Import, state upgrade, private state                  |
| `references/guides/validation.md`             | Attribute validators, resource-level validation       |
| `references/guides/testing.md`                | Unit tests, httptest fakes, live acceptance tests     |
| `references/guides/provider-configuration.md` | Provider setup, client injection                      |
| `references/guides/functions.md`              | Provider-defined functions (Terraform 1.8+)           |

### Framework Reference

Key entry points in the [Terraform Plugin Framework docs](https://developer.hashicorp.com/terraform/plugin/framework):

| Topic                                                                                                                       | Contents                         |
| --------------------------------------------------------------------------------------------------------------------------- | -------------------------------- |
| [resources/index](https://developer.hashicorp.com/terraform/plugin/framework/resources)                                     | Resource interface, registration |
| [resources/create](https://developer.hashicorp.com/terraform/plugin/framework/resources/create)                             | Create method contract           |
| [resources/read](https://developer.hashicorp.com/terraform/plugin/framework/resources/read)                                 | Read method, refresh state       |
| [resources/update](https://developer.hashicorp.com/terraform/plugin/framework/resources/update)                             | Update method, in-place changes  |
| [resources/delete](https://developer.hashicorp.com/terraform/plugin/framework/resources/delete)                             | Delete method                    |
| [resources/configure](https://developer.hashicorp.com/terraform/plugin/framework/resources/configure)                       | Client injection into resources  |
| [resources/import](https://developer.hashicorp.com/terraform/plugin/framework/resources/import)                             | Import state support             |
| [resources/plan-modification](https://developer.hashicorp.com/terraform/plugin/framework/resources/plan-modification)       | Plan modifiers                   |
| [resources/state-upgrade](https://developer.hashicorp.com/terraform/plugin/framework/resources/state-upgrade)               | State upgrade for schema changes |
| [data-sources/index](https://developer.hashicorp.com/terraform/plugin/framework/data-sources)                               | Data source interface            |
| [handling-data/schemas](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/schemas)                   | Schema definition                |
| [handling-data/accessing-values](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/accessing-values) | Reading config/plan/state        |
| [handling-data/writing-state](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/writing-state)       | Writing to response state        |
| [handling-data/attributes/index](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes)       | All attribute types              |
| [handling-data/blocks/index](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/blocks)               | All block types                  |
| [handling-data/types/index](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/types)                 | Type system (Go value types)     |
| [validation](https://developer.hashicorp.com/terraform/plugin/framework/validation)                                         | Validation patterns              |
| [diagnostics](https://developer.hashicorp.com/terraform/plugin/framework/diagnostics)                                       | Error/warning diagnostics        |
| [acctests](https://developer.hashicorp.com/terraform/plugin/framework/acctests)                                             | Acceptance testing setup         |
| [debugging](https://developer.hashicorp.com/terraform/plugin/framework/debugging)                                           | Debugging providers              |
| [providers/index](https://developer.hashicorp.com/terraform/plugin/framework/providers)                                     | Provider interface               |
| [provider-servers](https://developer.hashicorp.com/terraform/plugin/framework/provider-servers)                             | Provider server (main.go)        |
| [functions/implementation](https://developer.hashicorp.com/terraform/plugin/framework/functions/implementation)             | Provider functions               |
| [migrating/index](https://developer.hashicorp.com/terraform/plugin/framework/migrating)                                     | SDKv2 migration overview         |
