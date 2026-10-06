package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &projectResource{}
	_ resource.ResourceWithImportState = &projectResource{}
)

type projectID string

func (id projectID) path() string {
	return "/organization/projects/" + url.PathEscape(string(id))
}

type projectStatus string

const projectStatusArchived projectStatus = "archived"

type project struct {
	ID        projectID     `json:"id"`
	Name      string        `json:"name"`
	CreatedAt int64         `json:"created_at"`
	Status    projectStatus `json:"status"`
}

func (p *project) isArchived() bool {
	return p.Status == projectStatusArchived
}

type projectCreateRequest struct {
	Name string `json:"name"`
}

type projectUpdateRequest struct {
	Name string `json:"name"`
}

func newProject() resource.Resource { return &projectResource{} }

type projectResource struct {
	client *adminClient
}

type projectResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	CreatedAt types.String `tfsdk:"created_at"`
	Status    types.String `tfsdk:"status"`
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages an OpenAI project through the Admin API.

The Admin API can create, rename and archive a project, but it cannot delete
one. Running ` + "`terraform destroy`" + ` on this resource therefore archives the
project instead of deleting it. An archived project cannot be used or updated,
and it stays in the organization. A project archived or removed outside
Terraform is removed from state and created again on the next apply.`,
		Attributes: map[string]schema.Attribute{
			"id": rsID(),
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the project. It appears in reporting. Changing it renames the project in place.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 timestamp of when the project was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The project status, `active` or `archived`. A project in state is always `active`, since an archived project is removed from state on refresh.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*adminClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *adminClient, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created project
	if err := r.client.do(ctx, http.MethodPost, "/organization/projects", projectCreateRequest{Name: plan.Name.ValueString()}, &created); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create project: %s", err))
		return
	}

	applyProject(&plan, &created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.get(ctx, projectID(state.ID.ValueString()))
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read project: %s", err))
		return
	}
	if found.isArchived() {
		resp.State.RemoveResource(ctx)
		return
	}

	applyProject(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated project
	id := projectID(state.ID.ValueString())
	if err := r.client.do(ctx, http.MethodPost, id.path(), projectUpdateRequest{Name: plan.Name.ValueString()}, &updated); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update project: %s", err))
		return
	}

	applyProject(&plan, &updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := projectID(state.ID.ValueString())
	found, err := r.get(ctx, id)
	if isNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read project: %s", err))
		return
	}
	if found.isArchived() {
		return
	}

	if err := r.client.do(ctx, http.MethodPost, id.path()+"/archive", nil, nil); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to archive project: %s", err))
		return
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *projectResource) get(ctx context.Context, id projectID) (*project, error) {
	var found project
	if err := r.client.do(ctx, http.MethodGet, id.path(), nil, &found); err != nil {
		return nil, err
	}
	return &found, nil
}

func applyProject(model *projectResourceModel, found *project) {
	model.ID = types.StringValue(string(found.ID))
	model.Name = types.StringValue(found.Name)
	model.CreatedAt = unixTimestampValue(found.CreatedAt)
	model.Status = types.StringValue(string(found.Status))
}
