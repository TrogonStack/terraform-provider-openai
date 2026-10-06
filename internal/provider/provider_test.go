package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"openai": providerserver.NewProtocol6WithError(New("test")()),
}

func setupTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// testProviderConfig points the provider at a fake Admin API served under the
// same /v1 prefix as the default base URL.
func testProviderConfig(server *httptest.Server) string {
	return `
provider "openai" {
  admin_api_key = "` + testAdminAPIKey + `"
  base_url      = "` + server.URL + `/v1"
}
`
}

func configureProvider(t *testing.T, attributes map[string]tftypes.Value) *provider.ConfigureResponse {
	t.Helper()
	ctx := context.Background()
	p := New("test")()

	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema has errors: %v", schemaResp.Diagnostics)
	}

	values := map[string]tftypes.Value{
		"admin_api_key": tftypes.NewValue(tftypes.String, nil),
		"base_url":      tftypes.NewValue(tftypes.String, nil),
	}
	for name, value := range attributes {
		values[name] = value
	}
	raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), values)

	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	return resp
}

func TestProviderMetadata(t *testing.T) {
	p := New("1.2.3")()
	resp := &provider.MetadataResponse{}
	p.Metadata(context.Background(), provider.MetadataRequest{}, resp)
	if resp.TypeName != "openai" {
		t.Fatalf("expected type name openai, got %s", resp.TypeName)
	}
	if resp.Version != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %s", resp.Version)
	}
}

func TestProviderServes(t *testing.T) {
	server, err := testAccProtoV6ProviderFactories["openai"]()
	if err != nil {
		t.Fatal(err)
	}
	schema, err := server.GetProviderSchema(t.Context(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range schema.Diagnostics {
		t.Errorf("schema diagnostic: %s: %s", diagnostic.Summary, diagnostic.Detail)
	}
	for _, name := range []string{"openai_project", "openai_project_service_account"} {
		if _, ok := schema.ResourceSchemas[name]; !ok {
			t.Errorf("resource %s is not registered", name)
		}
	}
}

func TestProviderConfigureFailsWithoutAdminAPIKey(t *testing.T) {
	t.Setenv("OPENAI_ADMIN_KEY", "")
	resp := configureProvider(t, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected configure to fail without an Admin API key")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "OPENAI_ADMIN_KEY") || !strings.Contains(detail, "admin_api_key") {
		t.Fatalf("expected the error to name both admin_api_key and OPENAI_ADMIN_KEY, got: %s", detail)
	}
}

func TestProviderConfigureRejectsUnknownAdminAPIKey(t *testing.T) {
	resp := configureProvider(t, map[string]tftypes.Value{
		"admin_api_key": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected configure to fail with an unknown Admin API key")
	}
}

func TestProviderConfigureSharesTheClient(t *testing.T) {
	t.Setenv("OPENAI_ADMIN_KEY", "")
	resp := configureProvider(t, map[string]tftypes.Value{
		"admin_api_key": tftypes.NewValue(tftypes.String, testAdminAPIKey),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected configure to succeed, got %v", resp.Diagnostics)
	}
	client, ok := resp.ResourceData.(*adminClient)
	if !ok || client == nil {
		t.Fatalf("expected an *adminClient, got %#v", resp.ResourceData)
	}
	if resp.DataSourceData != resp.ResourceData {
		t.Fatal("expected data sources and resources to share the client")
	}
	if client.baseURL != defaultBaseURL {
		t.Fatalf("expected the default base URL %s, got %s", defaultBaseURL, client.baseURL)
	}
}

func TestNewClientRequiresAnAdminAPIKey(t *testing.T) {
	t.Setenv("OPENAI_ADMIN_KEY", "")
	if _, err := newClient(clientConfig{}); !errors.Is(err, errMissingAdminAPIKey) {
		t.Fatalf("err = %v, want %v", err, errMissingAdminAPIKey)
	}
}

func TestNewClientSendsTheResolvedCredential(t *testing.T) {
	cases := []struct {
		name           string
		configured     clientConfig
		environmentKey string
		baseURLFromEnv bool
		wantKey        string
	}{
		{name: "configured key wins over the environment", configured: clientConfig{adminAPIKey: "configured-key"}, environmentKey: "environment-key", wantKey: "configured-key"},
		{name: "environment key", environmentKey: "environment-key", wantKey: "environment-key"},
		{name: "base url from the environment", environmentKey: "environment-key", baseURLFromEnv: true, wantKey: "environment-key"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("OPENAI_ADMIN_KEY", testCase.environmentKey)
			t.Setenv("OPENAI_BASE_URL", "")

			var gotAuthorization, gotPath string
			server := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuthorization = r.Header.Get("Authorization")
				gotPath = r.URL.Path
				writeJSON(w, http.StatusOK, map[string]any{"id": "proj_test", "object": "organization.project", "name": "Test", "created_at": 1, "status": "active"})
			}))

			configured := testCase.configured
			if testCase.baseURLFromEnv {
				t.Setenv("OPENAI_BASE_URL", server.URL+"/v1/")
			} else {
				configured.baseURL = server.URL + "/v1"
			}
			client, err := newClient(configured)
			if err != nil {
				t.Fatal(err)
			}

			var found project
			if err := client.do(t.Context(), http.MethodGet, projectID("proj_test").path(), nil, &found); err != nil {
				t.Fatal(err)
			}
			if want := "Bearer " + testCase.wantKey; gotAuthorization != want {
				t.Errorf("Authorization = %q, want %q", gotAuthorization, want)
			}
			if gotPath != "/v1/organization/projects/proj_test" {
				t.Errorf("path = %q, want /v1/organization/projects/proj_test", gotPath)
			}
		})
	}
}

func TestClientSurfacesTheAPIErrorMessage(t *testing.T) {
	server := setupTestServer(t, newFakeAdminAPI())
	client, err := newClient(clientConfig{adminAPIKey: "wrong-key", baseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	err = client.do(t.Context(), http.MethodGet, projectID("proj_missing").path(), nil, nil)
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.statusCode != http.StatusUnauthorized {
		t.Fatalf("expected a 401 API error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid admin API key") {
		t.Fatalf("expected the API error message in %q", err.Error())
	}
}
