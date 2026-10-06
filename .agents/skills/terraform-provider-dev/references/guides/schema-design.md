# Schema Design

## Overview

Schemas define the shape of configuration, plan, and state data. Each attribute or block maps to a Go struct field via `tfsdk` tags.

No resource exists in this provider yet. The example below is illustrative, for a hypothetical `example_widget` resource:

```go
type widgetResourceModel struct {
    ID   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}
```

## Attribute Types

### Primitives

| Schema Type               | Go Type         | Notes               |
| ------------------------- | --------------- | ------------------- |
| `schema.StringAttribute`  | `types.String`  | UTF-8 string        |
| `schema.BoolAttribute`    | `types.Bool`    | true/false          |
| `schema.Int64Attribute`   | `types.Int64`   | 64-bit integer      |
| `schema.Int32Attribute`   | `types.Int32`   | 32-bit integer      |
| `schema.Float64Attribute` | `types.Float64` | 64-bit float        |
| `schema.Float32Attribute` | `types.Float32` | 32-bit float        |
| `schema.NumberAttribute`  | `types.Number`  | Arbitrary precision |

No attribute exists in the codebase yet; these types become relevant with the first provider setting or resource.

### Collections

| Schema Type            | Go Type      | Requires      |
| ---------------------- | ------------ | ------------- |
| `schema.ListAttribute` | `types.List` | `ElementType` |
| `schema.MapAttribute`  | `types.Map`  | `ElementType` |
| `schema.SetAttribute`  | `types.Set`  | `ElementType` |

Not used anywhere in this provider yet. A future resource modeling something the API returns as a map keyed by region or tag would likely reach for `schema.MapNestedAttribute` rather than a flat collection, since each value is itself an object.

### Nested Attributes (Protocol v6 only)

| Schema Type                    | Go Type                  | Use Case                |
| ------------------------------ | ------------------------ | ----------------------- |
| `schema.SingleNestedAttribute` | `*nestedModel`           | Single object           |
| `schema.ListNestedAttribute`   | `[]nestedModel`          | Ordered list of objects |
| `schema.MapNestedAttribute`    | `map[string]nestedModel` | Keyed objects           |
| `schema.SetNestedAttribute`    | `[]nestedModel`          | Unique set of objects   |

Not used anywhere in this provider yet:

```go
schema.SingleNestedAttribute{
    Optional: true,
    Attributes: map[string]schema.Attribute{
        "key":   schema.StringAttribute{Required: true},
        "value": schema.StringAttribute{Required: true},
    },
}
```

## Blocks

Blocks are structural containers that appear as HCL blocks (with `{}` syntax). Use blocks for complex nested structures, especially when they can be optional or repeated.

| Schema Type                | Go Type                               | HCL Syntax                              |
| -------------------------- | ------------------------------------- | --------------------------------------- |
| `schema.SingleNestedBlock` | `*nestedModel` (pointer for optional) | `block_name { ... }`                    |
| `schema.ListNestedBlock`   | `[]nestedModel`                       | `block_name { ... }` (repeated)         |
| `schema.SetNestedBlock`    | `[]nestedModel`                       | `block_name { ... }` (unique, repeated) |

Not used anywhere in this provider yet; reach for a block only if a future resource needs an optional, HCL-block-shaped nested structure.

### Blocks vs Nested Attributes

| Use Blocks When                                      | Use Nested Attributes When             |
| ---------------------------------------------------- | -------------------------------------- |
| Optional complex object (pointer nil = not provided) | Always-present object structure        |
| Matching existing Terraform provider conventions     | New providers (preferred direction)    |
| HCL block syntax feels natural for the structure     | Programmatic, data-oriented structures |

## Attribute Behaviors

### Required, Optional, Computed

| Combination                                    | Meaning                                    |
| ---------------------------------------------- | ------------------------------------------ |
| `Required: true`                               | User must provide; error if missing        |
| `Optional: true`                               | User may provide; null if omitted          |
| `Computed: true`                               | Provider sets the value; user cannot       |
| `Optional: true, Computed: true`               | User may provide OR provider fills         |
| `Optional: true, Computed: true, Default: ...` | User may provide; known default if omitted |

A hypothetical `example_widget` would mark `name` as `Required` and `id` as `Computed: true`, since the identifier is assigned by the server rather than something the caller sets directly.

### Sensitive

```go
schema.StringAttribute{
    Required:  true,
    Sensitive: true, // Value hidden in plan/state output
}
```

Not used anywhere in this provider yet; reach for it on any future attribute that carries a credential or secret value.

### Deprecation

```go
schema.StringAttribute{
    Optional:           true,
    DeprecationMessage: "Use 'new_field' instead.",
}
```

Not used anywhere in this provider yet.

## Defaults

Set a known value when the user does not provide one. Requires `Optional: true, Computed: true`.

```go
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"

schema.StringAttribute{
    Optional: true,
    Computed: true,
    Default:  stringdefault.StaticString("default-value"),
}
```

Not used anywhere in this provider yet; the signature above is illustrative, for a hypothetical `example_widget.category` that defaults to `"default-value"` when not configured.

## Plan Modifiers

Control how attribute values change during planning.

```go
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

schema.StringAttribute{
    Computed: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(), // ID: stable after creation
    },
}

schema.StringAttribute{
    Required: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(), // Immutable: forces recreation
    },
}
```

A hypothetical `example_widget.name` would use `stringplanmodifier.RequiresReplace()` if the API has no operation to rename a widget in place, so changing it means a different widget, not an update to the current one. Full details: `references/guides/plan-modification.md`.

## Validators

Constrain acceptable values at plan time.

```go
import "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
import "github.com/hashicorp/terraform-plugin-framework/schema/validator"

schema.StringAttribute{
    Required: true,
    Validators: []validator.String{
        stringvalidator.OneOf("small", "medium", "large"),
    },
}
```

Not used anywhere in this provider yet; the signature above is illustrative, restricting a hypothetical `example_widget.size` to a fixed set of values. Full details: `references/guides/validation.md`.

## ID Attribute Pattern

This provider has no resources yet, so there is no `helpers.go` and no shared ID-attribute helper. For a future resource where the API returns a distinct ID, give it this shape directly:

```go
"id": schema.StringAttribute{
    Computed:            true,
    MarkdownDescription: "The unique ID of this resource.",
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(),
    },
}
```

If a second resource repeats the same shape verbatim, that is the point to extract it into a shared helper in a new `helpers.go`, following the pattern other Terraform providers in this organization use (a function, conventionally named `rsId()`, returning this attribute).

## Accessing Values from Models

```go
// Read plan/config into model
var plan widgetResourceModel
resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

// Access primitive values
name := plan.Name.ValueString()

// Check null/unknown
if plan.Name.IsNull() { /* user did not set */ }
if plan.ID.IsUnknown() { /* will be known after apply */ }

// Set values
plan.ID = types.StringValue("w-123")
```

## Related Framework References

| File                                                                                                                                        | Contents                              |
| ------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| [handling-data/schemas](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/schemas)                                   | Schema definition fundamentals        |
| [handling-data/attributes/index](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes)                       | All attribute types overview          |
| [handling-data/attributes/string](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes/string)               | String attribute details              |
| [handling-data/attributes/list-nested](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes/list-nested)     | List nested attribute                 |
| [handling-data/attributes/single-nested](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes/single-nested) | Single nested attribute               |
| [handling-data/blocks/index](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/blocks)                               | Block types overview                  |
| [handling-data/blocks/single-nested](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/blocks/single-nested)         | SingleNestedBlock details             |
| [handling-data/types/index](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/types)                                 | Go value types                        |
| [handling-data/accessing-values](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/accessing-values)                 | Reading values from state/plan/config |
| [handling-data/writing-state](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/writing-state)                       | Writing values to state               |
| [resources/default](https://developer.hashicorp.com/terraform/plugin/framework/resources/default)                                           | Default values                        |
