# Data Source Lifecycle

The provider does not define any data sources today (`DataSources()` in `provider.go` returns an empty slice). Everything below is the pattern to follow when one is added, built around a hypothetical `example_widget` data source, read by ID.

## Interface

A data source must implement `datasource.DataSource`:

```go
type DataSource interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Read(context.Context, ReadRequest, *ReadResponse)
}
```

Optional interfaces:

- `datasource.DataSourceWithConfigure`: receive provider client
- `datasource.DataSourceWithValidateConfig`: configuration validation

## Registration

```go
func newWidgetDataSource() datasource.DataSource { return &widgetDataSource{} }

// In provider.go:
func (p *exampleProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{
        newWidgetDataSource,
    }
}
```

## Metadata

```go
func (d *widgetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_widget"
}
```

## Schema

Data source schemas use `datasource/schema` package (not `resource/schema`):

```go
import "github.com/hashicorp/terraform-plugin-framework/datasource/schema"

func (d *widgetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":   schema.StringAttribute{Required: true},
            "name": schema.StringAttribute{Computed: true},
        },
    }
}
```

Key differences from resource schemas:

- No plan modifiers (no plan phase for data sources)
- No defaults (no apply phase)
- Attributes are either Required (lookup key) or Computed (returned value)
- Optional attributes serve as optional filter criteria

## Configure

Same pattern as resources:

```go
func (d *widgetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*http.Client)
    if !ok {
        resp.Diagnostics.AddError("Unexpected DataSource Configure Type",
            fmt.Sprintf("Expected *http.Client, got: %T", req.ProviderData))
        return
    }
    d.client = client
}
```

## Read

Contract:

- Read configuration from `req.Config` (the user-provided lookup criteria)
- Perform the API call to find the data
- If not found: add an error diagnostic (data sources must find their target)
- Set all attribute values in `resp.State`

A hypothetical get-by-ID endpoint, so a data source would make one request rather than filtering a list client-side:

```go
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

    if httpResp.StatusCode == http.StatusNotFound {
        resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Widget %q not found", data.ID.ValueString()))
        return
    }

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

A future data source backed by a list-only endpoint (no get-by-key) would instead call a list method and filter client-side, looping while the response's token-pagination field carries a next-page token, treating "not present in any page" the same way: an error diagnostic, not a state removal.

## Data Sources vs Resources

| Aspect           | Resource                     | Data Source          |
| ---------------- | ---------------------------- | -------------------- |
| Purpose          | Manage lifecycle (CRUD)      | Read-only lookup     |
| Methods          | Create, Read, Update, Delete | Read only            |
| Import           | Supported                    | N/A                  |
| Plan modifiers   | Yes                          | No                   |
| Defaults         | Yes                          | No                   |
| State management | Full lifecycle               | Refreshed every plan |
| Not found        | RemoveResource (drift)       | Error diagnostic     |

## Related Framework References

| File                                                                                                                                  | Contents                            |
| ------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| [data-sources/index](https://developer.hashicorp.com/terraform/plugin/framework/data-sources)                                         | Data source interface, registration |
| [data-sources/configure](https://developer.hashicorp.com/terraform/plugin/framework/data-sources/configure)                           | Configure method                    |
| [data-sources/validate-configuration](https://developer.hashicorp.com/terraform/plugin/framework/data-sources/validate-configuration) | Validation                          |
| [data-sources/timeouts](https://developer.hashicorp.com/terraform/plugin/framework/data-sources/timeouts)                             | Timeout support                     |
