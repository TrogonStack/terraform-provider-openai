# Resource Lifecycle

This provider has no resources yet (`Resources()` in `provider.go` returns an empty slice). Everything below is illustrative, built around a hypothetical `example_widget` resource, grounded in the real shared `*http.Client` and `retry.go` plumbing that already ships.

## Interface

A resource must implement `resource.Resource`:

```go
type Resource interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Create(context.Context, CreateRequest, *CreateResponse)
    Read(context.Context, ReadRequest, *ReadResponse)
    Update(context.Context, UpdateRequest, *UpdateResponse)
    Delete(context.Context, DeleteRequest, *DeleteResponse)
}
```

Optional interfaces:

- `resource.ResourceWithConfigure`: receive provider client
- `resource.ResourceWithImportState`: support `terraform import`
- `resource.ResourceWithUpgradeState`: handle schema migrations
- `resource.ResourceWithModifyPlan`: resource-level plan modification
- `resource.ResourceWithValidateConfig`: resource-level validation

## Registration

Add a constructor function to the provider's `Resources()` method:

```go
func newWidgetResource() resource.Resource { return &widgetResource{} }

// In provider.go:
func (p *exampleProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{
        newWidgetResource,
    }
}
```

## Metadata

Sets the resource type name as it appears in Terraform configurations:

```go
func (r *widgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_widget"
}
```

This produces `example_widget` as the resource type (`req.ProviderTypeName` is `"example"`, set in the provider's own `Metadata`).

## Configure

Receive the provider-configured client:

```go
func (r *widgetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*http.Client)
    if !ok {
        resp.Diagnostics.AddError("Unexpected Resource Configure Type",
            fmt.Sprintf("Expected *http.Client, got: %T", req.ProviderData))
        return
    }
    r.client = client
}
```

The `nil` check is required: Configure is called during validation when provider data is not yet available.

## Create

Contract:

- Read plan data from `req.Plan`
- Perform the API creation call
- Set ALL attribute values (including computed) in `resp.State`
- Unknown values in plan MUST become known in state (error otherwise)
- On error, the resource is marked tainted for recreation on next plan

```go
func (r *widgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var plan widgetResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    body, err := json.Marshal(map[string]string{"name": plan.Name.ValueString()})
    if err != nil {
        resp.Diagnostics.AddError("Request Error", fmt.Sprintf("Unable to encode request body: %s", err))
        return
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.example.com/widgets", bytes.NewReader(body))
    if err != nil {
        resp.Diagnostics.AddError("Request Error", fmt.Sprintf("Unable to build request: %s", err))
        return
    }
    httpReq.Header.Set("Content-Type", "application/json")

    httpResp, err := r.client.Do(httpReq)
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create widget: %s", err))
        return
    }
    defer httpResp.Body.Close()

    var created struct {
        ID   string `json:"id"`
        Name string `json:"name"`
    }
    if err := json.NewDecoder(httpResp.Body).Decode(&created); err != nil {
        resp.Diagnostics.AddError("Response Error", fmt.Sprintf("Unable to decode response: %s", err))
        return
    }

    plan.ID = types.StringValue(created.ID)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
```

## Read

Contract:

- Read prior state from `req.State`
- Perform the API read call
- If the resource no longer exists: call `resp.State.RemoveResource(ctx)` and return
- Otherwise, update all state values to reflect current API state

A direct get-by-ID call, checking the response status code directly since there is no shared not-found helper yet:

```go
func (r *widgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state widgetResourceModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/widgets/"+state.ID.ValueString(), nil)
    if err != nil {
        resp.Diagnostics.AddError("Request Error", fmt.Sprintf("Unable to build request: %s", err))
        return
    }

    httpResp, err := r.client.Do(httpReq)
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read widget: %s", err))
        return
    }
    defer httpResp.Body.Close()

    if httpResp.StatusCode == http.StatusNotFound {
        resp.State.RemoveResource(ctx)
        return
    }

    var found struct {
        Name string `json:"name"`
    }
    if err := json.NewDecoder(httpResp.Body).Decode(&found); err != nil {
        resp.Diagnostics.AddError("Response Error", fmt.Sprintf("Unable to decode response: %s", err))
        return
    }

    state.Name = types.StringValue(found.Name)
    resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
```

For an endpoint that has no get-by-key (list-only), Read would call a list endpoint, looping while the response carries a next-page token, and filter client-side by key instead, collapsing "not found" and "present in state but absent from every page" into the same `RemoveResource` branch.

## Update

Contract:

- Read plan data from `req.Plan` (the desired new state)
- Perform the API update call
- Set state to reflect the actual post-update values
- All values in state MUST match plan values (or Terraform produces an "inconsistent result" error)

```go
func (r *widgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    var plan widgetResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    body, err := json.Marshal(map[string]string{"name": plan.Name.ValueString()})
    if err != nil {
        resp.Diagnostics.AddError("Request Error", fmt.Sprintf("Unable to encode request body: %s", err))
        return
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, "https://api.example.com/widgets/"+plan.ID.ValueString(), bytes.NewReader(body))
    if err != nil {
        resp.Diagnostics.AddError("Request Error", fmt.Sprintf("Unable to build request: %s", err))
        return
    }
    httpReq.Header.Set("Content-Type", "application/json")

    httpResp, err := r.client.Do(httpReq)
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update widget: %s", err))
        return
    }
    defer httpResp.Body.Close()

    var updated struct {
        Name string `json:"name"`
    }
    if err := json.NewDecoder(httpResp.Body).Decode(&updated); err != nil {
        resp.Diagnostics.AddError("Response Error", fmt.Sprintf("Unable to decode response: %s", err))
        return
    }

    plan.Name = types.StringValue(updated.Name)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
```

`name` is not expected to change here: it would be marked `RequiresReplace` in the schema (see `references/guides/plan-modification.md`) whenever the hypothetical API has no operation to rename a widget in place.

## Delete

Contract:

- Read prior state from `req.State`
- Perform the API deletion
- If already deleted: return without error (idempotent)
- No need to modify state: framework removes it automatically on success

```go
func (r *widgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
    var state widgetResourceModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, "https://api.example.com/widgets/"+state.ID.ValueString(), nil)
    if err != nil {
        resp.Diagnostics.AddError("Request Error", fmt.Sprintf("Unable to build request: %s", err))
        return
    }

    httpResp, err := r.client.Do(httpReq)
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete widget: %s", err))
        return
    }
    defer httpResp.Body.Close()

    if httpResp.StatusCode != http.StatusNotFound && httpResp.StatusCode >= 300 {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete widget, got status: %d", httpResp.StatusCode))
    }
}
```

Some APIs document no delete operation at all for a given object. For a resource built on one of those, Delete should return without error and document in the resource's schema description that destroy is a no-op, rather than inventing a delete call that doesn't exist.

## Decoding API Errors

Since there is no generated SDK, a non-2xx response comes back as an ordinary `*http.Response` with a status code the resource must check itself, not a typed error value. CRUD methods inspect `httpResp.StatusCode` directly:

```go
if httpResp.StatusCode == http.StatusNotFound {
    resp.State.RemoveResource(ctx)
    return
}
if httpResp.StatusCode >= 300 {
    resp.Diagnostics.AddError("API Error", fmt.Sprintf("request failed with status: %d", httpResp.StatusCode))
    return
}
```

Once a second resource needs the same status-code check, that is the point to extract a shared helper (conventionally named `isNotFound`) into a new `errors.go`.

## Related Framework References

| File                                                                                                  | Contents                                  |
| ----------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| [resources/index](https://developer.hashicorp.com/terraform/plugin/framework/resources)               | Resource type definition, full interface  |
| [resources/create](https://developer.hashicorp.com/terraform/plugin/framework/resources/create)       | Create method details and caveats         |
| [resources/read](https://developer.hashicorp.com/terraform/plugin/framework/resources/read)           | Read method and state refresh             |
| [resources/update](https://developer.hashicorp.com/terraform/plugin/framework/resources/update)       | Update method and plan consistency        |
| [resources/delete](https://developer.hashicorp.com/terraform/plugin/framework/resources/delete)       | Delete method                             |
| [resources/configure](https://developer.hashicorp.com/terraform/plugin/framework/resources/configure) | Configure method, provider data injection |
