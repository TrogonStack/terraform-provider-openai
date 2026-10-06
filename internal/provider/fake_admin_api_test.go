package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const testAdminAPIKey = "test-admin-key"

var fakeCreatedAt = time.Date(2024, 3, 26, 16, 45, 33, 0, time.UTC)

type fakeProject struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"created_at"`
	ArchivedAt *int64 `json:"archived_at"`
	Status     string `json:"status"`
}

// fakeAdminAPI is an in-memory stand-in for the Admin API project endpoints,
// shaped after the Project, ProjectServiceAccount and ErrorResponse schemas of
// the OpenAI OpenAPI specification.
type fakeAdminAPI struct {
	mu                   sync.Mutex
	nextProjectID        int
	projects             map[string]*fakeProject
	nextServiceAccountID int
	serviceAccounts      map[string]*fakeServiceAccount
	omitCreatedAPIKey    bool
	mux                  *http.ServeMux
}

func newFakeAdminAPI() *fakeAdminAPI {
	f := &fakeAdminAPI{
		projects: make(map[string]*fakeProject),
	}
	f.mux = http.NewServeMux()
	f.mux.HandleFunc("POST /v1/organization/projects", f.createProject)
	f.mux.HandleFunc("GET /v1/organization/projects/{project_id}", f.withProject(f.getProject))
	f.mux.HandleFunc("POST /v1/organization/projects/{project_id}", f.withProject(f.updateProject))
	f.mux.HandleFunc("POST /v1/organization/projects/{project_id}/archive", f.withProject(f.archiveProjectRoute))
	f.registerServiceAccountRoutes()
	f.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("%s %s is not served by the fake", r.Method, r.URL.Path))
	})
	return f
}

func (f *fakeAdminAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+testAdminAPIKey {
		writeAPIError(w, http.StatusUnauthorized, "invalid_request_error", "invalid admin API key")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mux.ServeHTTP(w, r)
}

func (f *fakeAdminAPI) withProject(handler func(http.ResponseWriter, *http.Request, *fakeProject)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := f.projects[r.PathValue("project_id")]
		if !ok {
			writeAPIError(w, http.StatusNotFound, "invalid_request_error", "Project not found")
			return
		}
		handler(w, r, project)
	}
}

func (f *fakeAdminAPI) newProject(name string) *fakeProject {
	f.nextProjectID++
	return &fakeProject{
		ID:        fmt.Sprintf("proj_%04d", f.nextProjectID),
		Object:    "organization.project",
		Name:      name,
		CreatedAt: fakeCreatedAt.Unix(),
		Status:    "active",
	}
}

func (f *fakeAdminAPI) createProject(w http.ResponseWriter, r *http.Request) {
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
	project := f.newProject(*body.Name)
	f.projects[project.ID] = project
	writeJSON(w, http.StatusOK, project)
}

func (f *fakeAdminAPI) getProject(w http.ResponseWriter, _ *http.Request, project *fakeProject) {
	writeJSON(w, http.StatusOK, project)
}

func (f *fakeAdminAPI) updateProject(w http.ResponseWriter, r *http.Request, project *fakeProject) {
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
	if project.Status == "archived" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "Archived projects cannot be updated")
		return
	}
	project.Name = *body.Name
	writeJSON(w, http.StatusOK, project)
}

func (f *fakeAdminAPI) archiveProjectRoute(w http.ResponseWriter, _ *http.Request, project *fakeProject) {
	f.archiveProject(project.ID)
	writeJSON(w, http.StatusOK, project)
}

// seedProject, archiveProject and removeProject simulate changes made outside
// Terraform. Callers outside ServeHTTP must hold mu.
func (f *fakeAdminAPI) seedProject(name string) string {
	project := f.newProject(name)
	f.projects[project.ID] = project
	return project.ID
}

func (f *fakeAdminAPI) archiveProject(id string) {
	project := f.projects[id]
	if project.Status == "archived" {
		return
	}
	archivedAt := fakeCreatedAt.Add(time.Hour).Unix()
	project.ArchivedAt = &archivedAt
	project.Status = "archived"
}

func (f *fakeAdminAPI) removeProject(id string) {
	delete(f.projects, id)
}

func decodeFakeRequest(w http.ResponseWriter, r *http.Request, body any) bool {
	if err := json.NewDecoder(r.Body).Decode(body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return false
	}
	return true
}

func writeAPIError(w http.ResponseWriter, status int, errorType, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"type":    errorType,
			"message": message,
			"param":   nil,
			"code":    nil,
		},
	})
}
