package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func rsID() schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The unique ID of this resource.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

func unixTimestampValue(seconds int64) types.String {
	if seconds == 0 {
		return types.StringNull()
	}
	return types.StringValue(time.Unix(seconds, 0).UTC().Format(time.RFC3339))
}

// keptFromCreate plans the value already in state, null included, for a
// computed attribute the API returns only in its create response. Unlike
// UseStateForUnknown it also keeps a null, so an imported resource does not
// plan a change it can never apply.
func keptFromCreate() planmodifier.String {
	return keptFromCreateModifier{}
}

type keptFromCreateModifier struct{}

func (m keptFromCreateModifier) Description(_ context.Context) string {
	return "Set only when Terraform creates the resource, then kept unchanged."
}

func (m keptFromCreateModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m keptFromCreateModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	resp.PlanValue = req.StateValue
}
