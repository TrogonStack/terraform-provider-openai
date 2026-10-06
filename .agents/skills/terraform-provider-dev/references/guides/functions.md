# Provider-Defined Functions

## Overview

Provider-defined functions (Terraform 1.8+) let practitioners call provider logic directly in expressions. Unlike resources/data sources, functions are pure computations: no state, no side effects.

This provider defines no functions today. Everything below is illustrative, built around a plausible one: splitting a dotted widget name (e.g. `team-a.premium`) into its namespace and final segment, so a practitioner could do this in an expression:

```hcl
# Usage in Terraform config (illustrative, this function does not exist):
output "widget_suffix" {
  value = provider::example::widget_name_suffix("team-a.premium").suffix
}
```

## Interface

```go
type Function interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Definition(context.Context, DefinitionRequest, *DefinitionResponse)
    Run(context.Context, RunRequest, *RunResponse)
}
```

## Implementation

### Define the Function

```go
package provider

import (
    "context"
    "fmt"
    "strings"

    "github.com/hashicorp/terraform-plugin-framework/function"
    "github.com/hashicorp/terraform-plugin-framework/types"
)

var _ function.Function = &widgetNameSuffixFunction{}

func newWidgetNameSuffixFunction() function.Function {
    return &widgetNameSuffixFunction{}
}

type widgetNameSuffixFunction struct{}

func (f *widgetNameSuffixFunction) Metadata(_ context.Context, req function.MetadataRequest, resp *function.MetadataResponse) {
    resp.Name = "widget_name_suffix"
}

func (f *widgetNameSuffixFunction) Definition(_ context.Context, req function.DefinitionRequest, resp *function.DefinitionResponse) {
    resp.Definition = function.Definition{
        Summary:     "Splits a dotted widget name into its namespace and final segment",
        Description: "Given a name like \"team-a.premium\", returns an object with namespace (\"team-a\") and suffix (\"premium\") attributes.",
        Parameters: []function.Parameter{
            function.StringParameter{
                Name:        "name",
                Description: "The widget name, e.g. \"team-a.premium\"",
            },
        },
        Return: function.ObjectReturn{
            AttributeTypes: map[string]attr.Type{
                "namespace": types.StringType,
                "suffix":    types.StringType,
            },
        },
    }
}

func (f *widgetNameSuffixFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
    var name string
    resp.Error = function.ConcatFuncErrors(req.Arguments.Get(ctx, &name))
    if resp.Error != nil {
        return
    }

    i := strings.LastIndex(name, ".")
    if i < 0 {
        resp.Error = function.NewArgumentFuncError(0, fmt.Sprintf("expected a dotted name such as \"team-a.premium\", got: %q", name))
        return
    }

    result, diags := types.ObjectValue(
        map[string]attr.Type{"namespace": types.StringType, "suffix": types.StringType},
        map[string]attr.Value{"namespace": types.StringValue(name[:i]), "suffix": types.StringValue(name[i+1:])},
    )
    resp.Error = function.ConcatFuncErrors(function.FuncErrorFromDiags(ctx, diags))
    if resp.Error != nil {
        return
    }
    resp.Error = function.ConcatFuncErrors(resp.Result.Set(ctx, result))
}
```

### Register with Provider

Add to the provider's `Functions` method:

```go
var _ provider.ProviderWithFunctions = &exampleProvider{}

func (p *exampleProvider) Functions(_ context.Context) []func() function.Function {
    return []func() function.Function{
        newWidgetNameSuffixFunction,
    }
}
```

`exampleProvider` does not implement `provider.ProviderWithFunctions` today; this would be a new addition to `provider.go`, alongside `Resources()` and `DataSources()`.

## Parameter Types

| Parameter Type              | Go Argument Type              |
| --------------------------- | ----------------------------- |
| `function.StringParameter`  | `string`                      |
| `function.BoolParameter`    | `bool`                        |
| `function.Int64Parameter`   | `int64`                       |
| `function.Float64Parameter` | `float64`                     |
| `function.ListParameter`    | `[]T` or `types.List`         |
| `function.MapParameter`     | `map[string]T` or `types.Map` |
| `function.SetParameter`     | `[]T` or `types.Set`          |
| `function.ObjectParameter`  | struct or `types.Object`      |
| `function.DynamicParameter` | `types.Dynamic`               |

### Variadic Parameter

```go
resp.Definition = function.Definition{
    Parameters: []function.Parameter{
        function.StringParameter{Name: "separator"},
    },
    VariadicParameter: function.StringParameter{
        Name:        "values",
        Description: "Values to join",
    },
    Return: function.StringReturn{},
}

func (f *joinFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
    var separator string
    var values []string
    resp.Error = function.ConcatFuncErrors(req.Arguments.Get(ctx, &separator, &values))
    // ...
}
```

## Return Types

| Return Type              | Go Result Type  |
| ------------------------ | --------------- |
| `function.StringReturn`  | `string`        |
| `function.BoolReturn`    | `bool`          |
| `function.Int64Return`   | `int64`         |
| `function.Float64Return` | `float64`       |
| `function.ListReturn`    | `types.List`    |
| `function.MapReturn`     | `types.Map`     |
| `function.SetReturn`     | `types.Set`     |
| `function.ObjectReturn`  | `types.Object`  |
| `function.DynamicReturn` | `types.Dynamic` |

## Error Handling

Functions use `function.FuncError` instead of diagnostics:

```go
// Single error
resp.Error = function.NewFuncError("something went wrong")

// Error with argument position
resp.Error = function.NewArgumentFuncError(0, "first argument is invalid")

// Combine errors
resp.Error = function.ConcatFuncErrors(
    req.Arguments.Get(ctx, &arg1, &arg2),
)
```

## Testing Functions

### Unit Tests

```go
func TestWidgetNameSuffixFunction(t *testing.T) {
    f := &widgetNameSuffixFunction{}

    // Test definition
    defResp := function.DefinitionResponse{}
    f.Definition(context.Background(), function.DefinitionRequest{}, &defResp)
    if defResp.Definition.Summary == "" {
        t.Error("expected non-empty summary")
    }
}
```

### Acceptance Tests

```go
resource.Test(t, resource.TestCase{
    ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
    Steps: []resource.TestStep{
        {
            Config: testProviderConfig + `
output "test" {
  value = provider::example::widget_name_suffix("team-a.premium").suffix
}
`,
            Check: resource.TestCheckOutput("test", "premium"),
        },
    },
})
```

## Related Framework References

| File                                                                                                            | Contents               |
| --------------------------------------------------------------------------------------------------------------- | ---------------------- |
| [functions/index](https://developer.hashicorp.com/terraform/plugin/framework/functions)                         | Functions overview     |
| [functions/concepts](https://developer.hashicorp.com/terraform/plugin/framework/functions/concepts)             | Concepts and use cases |
| [functions/implementation](https://developer.hashicorp.com/terraform/plugin/framework/functions/implementation) | Implementation details |
| [functions/testing](https://developer.hashicorp.com/terraform/plugin/framework/functions/testing)               | Testing functions      |
| [functions/errors](https://developer.hashicorp.com/terraform/plugin/framework/functions/errors)                 | Error handling         |
| [functions/parameters/index](https://developer.hashicorp.com/terraform/plugin/framework/functions/parameters)   | All parameter types    |
| [functions/returns/index](https://developer.hashicorp.com/terraform/plugin/framework/functions/returns)         | All return types       |
