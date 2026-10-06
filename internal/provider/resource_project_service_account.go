package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &projectServiceAccountResource{}
	_ resource.ResourceWithImportState = &projectServiceAccountResource{}
)

type serviceAccountID string

type projectServiceAccountID struct {
	project        projectID
	serviceAccount serviceAccountID
}

func parseProjectServiceAccountID(value string) (projectServiceAccountID, error) {
	project, serviceAccount, ok := strings.Cut(value, "/")
	if !ok || project == "" || serviceAccount == "" || strings.Contains(serviceAccount, "/") {
		return projectServiceAccountID{}, fmt.Errorf("expected an ID in the form <project_id>/<service_account_id>, got: %s", value)
	}
	return projectServiceAccountID{project: projectID(project), serviceAccount: serviceAccountID(serviceAccount)}, nil
}

func (id projectServiceAccountID) path() string {
	return id.project.path() + "/service_accounts/" + url.PathEscape(string(id.serviceAccount))
}

type projectServiceAccount struct {
	ID        serviceAccountID `json:"id"`
	Name      string           `json:"name"`
	Role      string           `json:"role"`
	CreatedAt int64            `json:"created_at"`
}

type projectServiceAccountAPIKey struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type projectServiceAccountCreateRequest struct {
	Name string `json:"name"`
}

type projectServiceAccountCreateResponse struct {
	projectServiceAccount
	APIKey *projectServiceAccountAPIKey `json:"api_key"`
}

func newProjectServiceAccount() resource.Resource { return &projectServiceAccountResource{} }

type projectServiceAccountResource struct {
	client *adminClient
}

type projectServiceAccountResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Name      types.String `tfsdk:"name"`
	Role      types.String `tfsdk:"role"`
	CreatedAt types.String `tfsdk:"created_at"`
	APIKeyID  types.String `tfsdk:"api_key_id"`
	APIKey    types.String `tfsdk:"api_key"`
}

func (model *projectServiceAccountResourceModel) identifier() projectServiceAccountID {
	return projectServiceAccountID{
		project:        projectID(model.ProjectID.ValueString()),
		serviceAccount: serviceAccountID(model.ID.ValueString()),
	}
}

func (r *projectServiceAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_service_account"
}

func (r *projectServiceAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages a service account in an OpenAI project through the Admin API.

Creating a service account also creates an API key for it. The Admin API
returns that key's secret value only once, in the response to the create
request, and never again. Terraform stores it in ` + "`api_key`" + `, so the value
is only available for a service account that Terraform itself created. A
service account brought in with ` + "`terraform import`" + ` has a null ` + "`api_key`" + `
and ` + "`api_key_id`" + `.

The Admin API cannot rename a service account or issue it a new key, so
changing ` + "`name`" + ` or ` + "`project_id`" + ` replaces the service account. To rotate
the key, replace the resource, for example with
` + "`terraform apply -replace=\"openai_project_service_account.example\"`" + `.

Destroying this resource deletes the service account. A service account
deleted outside Terraform, or one whose project was archived, is removed from
state and created again on the next apply.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the service account.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the project the service account belongs to. Changing it replaces the service account.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the service account. Changing it replaces the service account and its API key.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The service account's role in the project, `owner` or `member`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 timestamp of when the service account was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"api_key_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the API key created with the service account. Null for an imported service account.",
				PlanModifiers: []planmodifier.String{
					keptFromCreate(),
				},
			},
			"api_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The secret value of the API key created with the service account. Returned by the Admin API only when the service account is created, so it is null for an imported service account.",
				PlanModifiers: []planmodifier.String{
					keptFromCreate(),
				},
			},
		},
	}
}

func (r *projectServiceAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *projectServiceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectServiceAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	project := projectID(plan.ProjectID.ValueString())
	var created projectServiceAccountCreateResponse
	if err := r.client.do(ctx, http.MethodPost, project.path()+"/service_accounts", projectServiceAccountCreateRequest{Name: plan.Name.ValueString()}, &created); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create project service account: %s", err))
		return
	}
	if created.APIKey == nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("The response to creating service account %s has no api_key, so its key value cannot be recorded. Delete the service account in project %s and apply again.", created.ID, project))
		return
	}

	applyProjectServiceAccount(&plan, &created.projectServiceAccount)
	plan.APIKeyID = types.StringValue(created.APIKey.ID)
	plan.APIKey = types.StringValue(created.APIKey.Value)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectServiceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectServiceAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.identifier()
	var found projectServiceAccount
	err := r.client.do(ctx, http.MethodGet, id.path(), nil, &found)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		if r.projectGone(ctx, id.project) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read project service account: %s", err))
		return
	}

	applyProjectServiceAccount(&state, &found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only carries state forward: every configurable attribute requires
// replacement, so the Admin API is never asked to change a service account.
func (r *projectServiceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectServiceAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectServiceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectServiceAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.identifier()
	err := r.client.do(ctx, http.MethodDelete, id.path(), nil, nil)
	if err == nil || isNotFound(err) {
		return
	}
	if r.projectGone(ctx, id.project) {
		return
	}
	resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete project service account: %s", err))
}

func (r *projectServiceAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := parseProjectServiceAccountID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), string(id.project))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), string(id.serviceAccount))...)
}

// projectGone reports whether the project no longer exists or is archived.
// The Admin API documents an archived project as having no service accounts,
// so either way the service account is gone too.
func (r *projectServiceAccountResource) projectGone(ctx context.Context, id projectID) bool {
	var found project
	err := r.client.do(ctx, http.MethodGet, id.path(), nil, &found)
	if isNotFound(err) {
		return true
	}
	return err == nil && found.isArchived()
}

func applyProjectServiceAccount(model *projectServiceAccountResourceModel, found *projectServiceAccount) {
	model.ID = types.StringValue(string(found.ID))
	model.Name = types.StringValue(found.Name)
	model.Role = types.StringValue(found.Role)
	model.CreatedAt = unixTimestampValue(found.CreatedAt)
}
