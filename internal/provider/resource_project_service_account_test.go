package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const projectServiceAccountAddress = "openai_project_service_account.test"

var regexpCannotImportNonExistent = regexp.MustCompile(`Cannot import non-existent remote object`)

func testProjectServiceAccountConfig(serviceAccountName string) string {
	return testProjectConfig("example-project") + fmt.Sprintf(`
resource "openai_project_service_account" "test" {
  project_id = openai_project.test.id
  name       = %q
}
`, serviceAccountName)
}

func expectProjectServiceAccountAction(action plancheck.ResourceActionType) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction(projectServiceAccountAddress, action),
		},
	}
}

func checkFakeServiceAccountsDeleted(fake *fakeAdminAPI) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		for id := range fake.serviceAccounts {
			return fmt.Errorf("expected service account %s to be deleted on destroy", id)
		}
		return nil
	}
}

func captureAttribute(address, attribute string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		*into = s.RootModule().Resources[address].Primary.Attributes[attribute]
		return nil
	}
}

func checkAttributeUnchanged(address, attribute string, want *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		got := s.RootModule().Resources[address].Primary.Attributes[attribute]
		if got != *want {
			return fmt.Errorf("%s.%s = %q, want the value from creation %q", address, attribute, got, *want)
		}
		return nil
	}
}

func projectServiceAccountImportID(s *terraform.State) (string, error) {
	rs, ok := s.RootModule().Resources[projectServiceAccountAddress]
	if !ok {
		return "", fmt.Errorf("resource not found: %s", projectServiceAccountAddress)
	}
	return rs.Primary.Attributes["project_id"] + "/" + rs.Primary.ID, nil
}

func TestAccProjectServiceAccount_Basic(t *testing.T) {
	fake := newFakeAdminAPI()
	server := setupTestServer(t, fake)
	config := testProviderConfig(server) + testProjectServiceAccountConfig("example-service-account")

	var apiKey, apiKeyID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkFakeServiceAccountsDeleted(fake),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "id", "svc_acct_0001"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "project_id", "proj_0001"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "name", "example-service-account"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "role", "member"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "created_at", "2024-03-26T16:45:33Z"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "api_key_id", "key_0001"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "api_key", "fake-key-value-for-svc_acct_0001"),
					captureAttribute(projectServiceAccountAddress, "api_key", &apiKey),
					captureAttribute(projectServiceAccountAddress, "api_key_id", &apiKeyID),
				),
			},
			{
				RefreshState: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAttributeUnchanged(projectServiceAccountAddress, "api_key", &apiKey),
					checkAttributeUnchanged(projectServiceAccountAddress, "api_key_id", &apiKeyID),
				),
			},
			{
				Config:           config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAttributeUnchanged(projectServiceAccountAddress, "api_key", &apiKey),
					checkAttributeUnchanged(projectServiceAccountAddress, "api_key_id", &apiKeyID),
				),
			},
			{
				ResourceName:            projectServiceAccountAddress,
				ImportState:             true,
				ImportStateIdFunc:       projectServiceAccountImportID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"api_key", "api_key_id"},
			},
		},
	})
}

func TestAccProjectServiceAccount_ChangingNameReplaces(t *testing.T) {
	fake := newFakeAdminAPI()
	server := setupTestServer(t, fake)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkFakeServiceAccountsDeleted(fake),
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(server) + testProjectServiceAccountConfig("example-service-account"),
				Check:  resource.TestCheckResourceAttr(projectServiceAccountAddress, "api_key_id", "key_0001"),
			},
			{
				Config:           testProviderConfig(server) + testProjectServiceAccountConfig("example-service-account-renamed"),
				ConfigPlanChecks: expectProjectServiceAccountAction(plancheck.ResourceActionDestroyBeforeCreate),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "id", "svc_acct_0002"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "api_key_id", "key_0002"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "api_key", "fake-key-value-for-svc_acct_0002"),
					func(_ *terraform.State) error {
						fake.mu.Lock()
						defer fake.mu.Unlock()
						if _, exists := fake.serviceAccounts["svc_acct_0001"]; exists {
							return fmt.Errorf("the replaced service account svc_acct_0001 was not deleted")
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccProjectServiceAccount_GoneOutOfBand(t *testing.T) {
	cases := map[string]func(fake *fakeAdminAPI){
		"service account deleted": func(fake *fakeAdminAPI) { fake.removeServiceAccount("svc_acct_0001") },
		"project archived":        func(fake *fakeAdminAPI) { fake.archiveProject("proj_0001") },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeAdminAPI()
			server := setupTestServer(t, fake)

			fake.mu.Lock()
			projectID := fake.seedProject("example-project")
			fake.mu.Unlock()

			config := testProviderConfig(server) + fmt.Sprintf(`
resource "openai_project_service_account" "test" {
  project_id = %q
  name       = "example-service-account"
}
`, projectID)

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: config,
						Check:  resource.TestCheckResourceAttr(projectServiceAccountAddress, "id", "svc_acct_0001"),
					},
					{
						PreConfig: func() {
							fake.mu.Lock()
							defer fake.mu.Unlock()
							change(fake)
						},
						RefreshState:       true,
						ExpectNonEmptyPlan: true,
						Check: func(s *terraform.State) error {
							if _, exists := s.RootModule().Resources[projectServiceAccountAddress]; exists {
								return fmt.Errorf("expected %s to be removed from state on refresh", projectServiceAccountAddress)
							}
							return nil
						},
					},
				},
			})
		})
	}
}

func TestAccProjectServiceAccount_DeletedOutOfBandIsRecreated(t *testing.T) {
	fake := newFakeAdminAPI()
	server := setupTestServer(t, fake)
	config := testProviderConfig(server) + testProjectServiceAccountConfig("example-service-account")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkFakeServiceAccountsDeleted(fake),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr(projectServiceAccountAddress, "id", "svc_acct_0001"),
			},
			{
				PreConfig: func() {
					fake.mu.Lock()
					defer fake.mu.Unlock()
					fake.removeServiceAccount("svc_acct_0001")
				},
				Config:           config,
				ConfigPlanChecks: expectProjectServiceAccountAction(plancheck.ResourceActionCreate),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "id", "svc_acct_0002"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "api_key", "fake-key-value-for-svc_acct_0002"),
				),
			},
		},
	})
}

func TestAccProjectServiceAccount_ImportLeavesTheKeyNull(t *testing.T) {
	fake := newFakeAdminAPI()
	server := setupTestServer(t, fake)

	fake.mu.Lock()
	projectID := fake.seedProject("example-project")
	serviceAccountID := fake.seedServiceAccount(projectID, "created-elsewhere")
	fake.mu.Unlock()

	config := testProviderConfig(server) + fmt.Sprintf(`
import {
  to = openai_project_service_account.test
  id = "%s/%s"
}

resource "openai_project_service_account" "test" {
  project_id = %q
  name       = "created-elsewhere"
}
`, projectID, serviceAccountID, projectID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkFakeServiceAccountsDeleted(fake),
		Steps: []resource.TestStep{
			{
				Config:           config,
				ConfigPlanChecks: expectProjectServiceAccountAction(plancheck.ResourceActionNoop),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "id", serviceAccountID),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "project_id", projectID),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "role", "member"),
					resource.TestCheckNoResourceAttr(projectServiceAccountAddress, "api_key"),
					resource.TestCheckNoResourceAttr(projectServiceAccountAddress, "api_key_id"),
				),
			},
			{
				Config:           config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func TestAccProjectServiceAccount_RejectsInvalidImportID(t *testing.T) {
	fake := newFakeAdminAPI()
	server := setupTestServer(t, fake)

	for _, importID := range []string{"not-a-valid-id", "/svc_acct_0001", "proj_0001/", "proj_0001/svc_acct_0001/extra"} {
		t.Run(importID, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:        testProviderConfig(server) + testProjectServiceAccountConfig("example-service-account"),
						ResourceName:  projectServiceAccountAddress,
						ImportState:   true,
						ImportStateId: importID,
						ExpectError:   regexp.MustCompile(`Invalid Import ID`),
					},
				},
			})
		})
	}
}

func TestProjectServiceAccountDeleteToleratesMissingServiceAccount(t *testing.T) {
	cases := map[string]func(fake *fakeAdminAPI, projectID, serviceAccountID string){
		"service account already deleted": func(fake *fakeAdminAPI, _, serviceAccountID string) { fake.removeServiceAccount(serviceAccountID) },
		"project archived":                func(fake *fakeAdminAPI, projectID, _ string) { fake.archiveProject(projectID) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeAdminAPI()
			server := setupTestServer(t, fake)
			client, err := newClient(clientConfig{adminAPIKey: testAdminAPIKey, baseURL: server.URL + "/v1"})
			if err != nil {
				t.Fatal(err)
			}

			fake.mu.Lock()
			projectID := fake.seedProject("example-project")
			serviceAccountID := fake.seedServiceAccount(projectID, "example-service-account")
			change(fake, projectID, serviceAccountID)
			fake.mu.Unlock()

			ctx := context.Background()
			serviceAccountResource := &projectServiceAccountResource{client: client}
			schemaResponse := &frameworkresource.SchemaResponse{}
			serviceAccountResource.Schema(ctx, frameworkresource.SchemaRequest{}, schemaResponse)

			state := tfsdk.State{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)}
			diagnostics := state.Set(ctx, &projectServiceAccountResourceModel{
				ID:        types.StringValue(serviceAccountID),
				ProjectID: types.StringValue(projectID),
				Name:      types.StringValue("example-service-account"),
				Role:      types.StringValue("member"),
				CreatedAt: types.StringValue("2024-03-26T16:45:33Z"),
				APIKeyID:  types.StringNull(),
				APIKey:    types.StringNull(),
			})
			if diagnostics.HasError() {
				t.Fatalf("unable to build state: %v", diagnostics)
			}

			response := &frameworkresource.DeleteResponse{State: state}
			serviceAccountResource.Delete(ctx, frameworkresource.DeleteRequest{State: state}, response)
			if response.Diagnostics.HasError() {
				t.Fatalf("expected delete to succeed, got %v", response.Diagnostics)
			}
		})
	}
}

func TestParseProjectServiceAccountID(t *testing.T) {
	id, err := parseProjectServiceAccountID("proj_abc/svc_acct_abc")
	if err != nil {
		t.Fatal(err)
	}
	if id.project != "proj_abc" || id.serviceAccount != "svc_acct_abc" {
		t.Fatalf("parsed %+v, want project proj_abc and service account svc_acct_abc", id)
	}
	if got := id.path(); got != "/organization/projects/proj_abc/service_accounts/svc_acct_abc" {
		t.Fatalf("path = %s", got)
	}
}
