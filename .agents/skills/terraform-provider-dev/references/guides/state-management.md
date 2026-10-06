# State Management

## Import

Import lets practitioners bring existing resources under Terraform management without recreating them. This provider has no resources yet, so nothing implements `resource.ResourceWithImportState` today. The shapes below are illustrative.

### Simple Import (PassthroughID)

When the import ID is the same as the resource's `id` attribute, which applies to a future resource with a single opaque ID of its own, such as a hypothetical `example_widget`:

```go
var _ resource.ResourceWithImportState = &widgetResource{}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
```

Usage: `terraform import example_widget.example "w-123"`

After `ImportState` sets the minimal attributes, Terraform calls Read to fill in the rest.

### Compound Import (Custom Parsing)

A resource keyed by more than one field needs both values, parsed by hand from a compound ID with a small helper type and function:

```go
type widgetImportID struct {
    Namespace string
    Name      string
}

func parseWidgetImportID(raw string) (widgetImportID, error) {
    parts := strings.SplitN(raw, "/", 2)
    if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
        return widgetImportID{}, fmt.Errorf("expected import ID in the format <namespace>/<name>, got: %q", raw)
    }
    return widgetImportID{Namespace: parts[0], Name: parts[1]}, nil
}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    parsed, err := parseWidgetImportID(req.ID)
    if err != nil {
        resp.Diagnostics.AddError("Invalid Import ID", err.Error())
        return
    }
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), parsed.Namespace)...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parsed.Name)...)
}
```

Usage: `terraform import example_widget.example "team-a/premium"`

## State Upgrade

When you change a resource schema in a breaking way, existing state in `.tfstate` files won't match the new schema. State upgraders transform old state to the new format transparently. No resource exists yet, so none has needed one.

### When to Use

- Changing a list block to SingleNestedBlock
- Renaming attributes
- Changing attribute types (e.g., string to int)
- Restructuring nested objects

### Implementation

1. Increment `Version` in the schema:

```go
func (r *widgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Version: 1, // Was 0, now 1
        // ... current schema ...
    }
}
```

2. Implement `resource.ResourceWithUpgradeState`:

```go
var _ resource.ResourceWithUpgradeState = &widgetResource{}

func (r *widgetResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
                // Parse raw JSON from old state format
                var raw map[string]json.RawMessage
                if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
                    resp.Diagnostics.AddError("State Upgrade Error",
                        fmt.Sprintf("Unable to parse raw state: %s", err))
                    return
                }

                var name string
                _ = json.Unmarshal(raw["name"], &name)

                state := widgetResourceModel{
                    Name: types.StringValue(name),
                }
                resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
            },
        },
    }
}
```

### Key Points

- The map key is the OLD schema version (upgrade FROM version X)
- `req.RawState.JSON` contains the raw JSON bytes of the old state
- Parse manually: the old state shape does not match your current model struct
- After upgrade, Terraform calls Read to refresh state with current API data
- Multiple upgraders can be chained (0->1, 1->2, etc.)

## Private State

Store provider-internal data that is not visible in plan output. Useful for:

- ETags or version tokens for optimistic concurrency
- Internal identifiers that shouldn't be user-visible
- Cached metadata to avoid extra API calls

Not used anywhere in this provider today, since there is no resource yet with a need for it. The pattern, if it's ever needed:

```go
var _ resource.ResourceWithPrivateState = &fooResource{}

// In Create or Update:
resp.Private.SetKey(ctx, "etag", []byte(apiResponse.Etag))

// In Read or Update:
etagBytes, diags := req.Private.GetKey(ctx, "etag")
etag := string(etagBytes)
```

## Writing State

### Full Model Write

Most common: write the entire model struct to state:

```go
resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
```

### Individual Attribute Write

Set a single attribute by path. The compound `ImportState` implementation above uses this to populate `namespace` and `name` before Terraform's follow-up Read:

```go
resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), parsed.Namespace)...)
```

### Removing Resource from State

When Read discovers the resource no longer exists, checking the response status code directly since there is no shared not-found helper yet (see `references/guides/resource-lifecycle.md`):

```go
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
```

This tells Terraform the resource was deleted externally and needs recreation.

## Related Framework References

| File                                                                                                                        | Contents                          |
| --------------------------------------------------------------------------------------------------------------------------- | --------------------------------- |
| [resources/import](https://developer.hashicorp.com/terraform/plugin/framework/resources/import)                             | Import state documentation        |
| [resources/state-upgrade](https://developer.hashicorp.com/terraform/plugin/framework/resources/state-upgrade)               | State upgrade details             |
| [resources/private-state](https://developer.hashicorp.com/terraform/plugin/framework/resources/private-state)               | Private state storage             |
| [resources/state-move](https://developer.hashicorp.com/terraform/plugin/framework/resources/state-move)                     | State move between resource types |
| [handling-data/writing-state](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/writing-state)       | Writing to response state         |
| [handling-data/accessing-values](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/accessing-values) | Reading from state/plan/config    |
