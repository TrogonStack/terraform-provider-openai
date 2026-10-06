package provider

import (
	"fmt"
	"net/http"
)

type fakeServiceAccount struct {
	Object    string `json:"object"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"`
	ProjectID string `json:"-"`
	APIKeyID  string `json:"-"`
}

type fakeServiceAccountAPIKey struct {
	Object    string `json:"object"`
	Value     string `json:"value"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	ID        string `json:"id"`
}

type fakeServiceAccountCreateResponse struct {
	*fakeServiceAccount
	APIKey fakeServiceAccountAPIKey `json:"api_key"`
}

// registerServiceAccountRoutes adds the project service account endpoints to
// the fake's mux. Called once from newFakeAdminAPI.
func (f *fakeAdminAPI) registerServiceAccountRoutes() {
	f.serviceAccounts = make(map[string]*fakeServiceAccount)

	f.mux.HandleFunc("POST /v1/organization/projects/{project_id}/service_accounts", f.withActiveProject(f.createServiceAccount))
	f.mux.HandleFunc("GET /v1/organization/projects/{project_id}/service_accounts/{service_account_id}", f.withActiveProject(f.withServiceAccount(f.getServiceAccount)))
	f.mux.HandleFunc("DELETE /v1/organization/projects/{project_id}/service_accounts/{service_account_id}", f.withActiveProject(f.withServiceAccount(f.deleteServiceAccount)))
}

// withActiveProject rejects calls on an archived project with a 400, as the
// specification documents for creating and listing service accounts there.
func (f *fakeAdminAPI) withActiveProject(handler func(http.ResponseWriter, *http.Request, *fakeProject)) http.HandlerFunc {
	return f.withProject(func(w http.ResponseWriter, r *http.Request, project *fakeProject) {
		if project.Status == "archived" {
			writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "Project is archived")
			return
		}
		handler(w, r, project)
	})
}

func (f *fakeAdminAPI) withServiceAccount(handler func(http.ResponseWriter, *http.Request, *fakeServiceAccount)) func(http.ResponseWriter, *http.Request, *fakeProject) {
	return func(w http.ResponseWriter, r *http.Request, project *fakeProject) {
		serviceAccount, ok := f.serviceAccounts[r.PathValue("service_account_id")]
		if !ok || serviceAccount.ProjectID != project.ID {
			writeAPIError(w, http.StatusNotFound, "invalid_request_error", "Service account not found")
			return
		}
		handler(w, r, serviceAccount)
	}
}

func (f *fakeAdminAPI) newServiceAccount(projectID, name string) *fakeServiceAccount {
	f.nextServiceAccountID++
	serviceAccount := &fakeServiceAccount{
		Object:    "organization.project.service_account",
		ID:        fmt.Sprintf("svc_acct_%04d", f.nextServiceAccountID),
		Name:      name,
		Role:      "member",
		CreatedAt: fakeCreatedAt.Unix(),
		ProjectID: projectID,
		APIKeyID:  fmt.Sprintf("key_%04d", f.nextServiceAccountID),
	}
	f.serviceAccounts[serviceAccount.ID] = serviceAccount
	return serviceAccount
}

func fakeServiceAccountKeyValue(serviceAccount *fakeServiceAccount) string {
	return "fake-key-value-for-" + serviceAccount.ID
}

func (f *fakeAdminAPI) createServiceAccount(w http.ResponseWriter, r *http.Request, project *fakeProject) {
	var body struct {
		Name *string `json:"name"`
	}
	if !decodeFakeRequest(w, r, &body) {
		return
	}
	if body.Name == nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "name is required")
		return
	}

	serviceAccount := f.newServiceAccount(project.ID, *body.Name)
	writeJSON(w, http.StatusOK, fakeServiceAccountCreateResponse{
		fakeServiceAccount: serviceAccount,
		APIKey: fakeServiceAccountAPIKey{
			Object:    "organization.project.service_account.api_key",
			Value:     fakeServiceAccountKeyValue(serviceAccount),
			Name:      "Secret Key",
			CreatedAt: serviceAccount.CreatedAt,
			ID:        serviceAccount.APIKeyID,
		},
	})
}

func (f *fakeAdminAPI) getServiceAccount(w http.ResponseWriter, _ *http.Request, serviceAccount *fakeServiceAccount) {
	writeJSON(w, http.StatusOK, serviceAccount)
}

func (f *fakeAdminAPI) deleteServiceAccount(w http.ResponseWriter, _ *http.Request, serviceAccount *fakeServiceAccount) {
	delete(f.serviceAccounts, serviceAccount.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"object":  "organization.project.service_account.deleted",
		"id":      serviceAccount.ID,
		"deleted": true,
	})
}

// seedServiceAccount and removeServiceAccount simulate changes made outside
// Terraform. Callers outside ServeHTTP must hold mu.
func (f *fakeAdminAPI) seedServiceAccount(projectID, name string) string {
	return f.newServiceAccount(projectID, name).ID
}

func (f *fakeAdminAPI) removeServiceAccount(id string) {
	delete(f.serviceAccounts, id)
}
